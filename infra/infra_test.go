// Package infra_test checks the CloudFormation templates against the agreed
// deployment rules. Templates use long-form intrinsics ({"Ref": ...}), not
// YAML short tags (!Ref), so they parse as plain YAML here.
package infra_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type template struct {
	Parameters map[string]map[string]any `yaml:"Parameters"`
	Resources  map[string]resource       `yaml:"Resources"`
}

type resource struct {
	Type         string         `yaml:"Type"`
	Properties   map[string]any `yaml:"Properties"`
	UpdatePolicy map[string]any `yaml:"UpdatePolicy"`
}

func load(t *testing.T, name string) template {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var tpl template
	if err := yaml.Unmarshal(b, &tpl); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return tpl
}

func (tpl template) ofType(typ string) map[string]resource {
	out := map[string]resource{}
	for id, r := range tpl.Resources {
		if r.Type == typ {
			out[id] = r
		}
	}
	return out
}

func (tpl template) only(t *testing.T, typ string) resource {
	t.Helper()
	rs := tpl.ofType(typ)
	if len(rs) != 1 {
		t.Fatalf("want exactly one %s, found %d", typ, len(rs))
	}
	for _, r := range rs {
		return r
	}
	return resource{}
}

// get walks nested maps/lists: get(v, "A", "B", 0, "C").
func get(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("at %v: %T is not a map", path, v)
			}
			v = m[k]
		case int:
			l, ok := v.([]any)
			if !ok || k >= len(l) {
				t.Fatalf("at %v: %T is not a list with index %d", path, v, k)
			}
			v = l[k]
		}
	}
	return v
}

func str(v any) string { return fmt.Sprint(v) }

// userData returns the instance start-up script from the launch template.
func userData(t *testing.T, app template) string {
	t.Helper()
	lt := app.only(t, "AWS::EC2::LaunchTemplate")
	s, ok := get(t, lt.Properties, "LaunchTemplateData", "UserData", "Fn::Base64").(string)
	if !ok {
		t.Fatal("UserData must be {Fn::Base64: <plain script>} so it can be linted")
	}
	return s
}

// TestAppRunsExactlyOneBot ensures there's never a second copy of the bot
// (two copies both answer every Discord click): the Auto Scaling Group holds
// exactly one instance, capacity rebalancing (which starts a replacement
// before stopping the old Spot instance) is off, and updates terminate the
// old instance before launching the new one.
func TestAppRunsExactlyOneBot(t *testing.T) {
	asg := load(t, "app.yml").only(t, "AWS::AutoScaling::AutoScalingGroup")
	for _, k := range []string{"MinSize", "MaxSize", "DesiredCapacity"} {
		if got := str(asg.Properties[k]); got != "1" {
			t.Errorf("%s = %s, want 1", k, got)
		}
	}
	if got := str(asg.Properties["CapacityRebalance"]); got != "false" {
		t.Errorf("CapacityRebalance = %s, want false", got)
	}
	if got := str(get(t, asg.UpdatePolicy, "AutoScalingRollingUpdate", "MinInstancesInService")); got != "0" {
		t.Errorf("rolling update MinInstancesInService = %s, want 0", got)
	}
}

// TestAppUsesOnDemandGraviton pins the compute decisions: one On-Demand
// t4g.micro (Spot is denied by the organization's SCP), the latest Amazon
// Linux 2023 ARM AMI, an 8 GiB gp3 root volume, and subnets in two
// availability zones.
func TestAppUsesOnDemandGraviton(t *testing.T) {
	app := load(t, "app.yml")
	asg := app.only(t, "AWS::AutoScaling::AutoScalingGroup")
	if _, ok := asg.Properties["MixedInstancesPolicy"]; ok {
		t.Error("ASG has a MixedInstancesPolicy (Spot), want a plain On-Demand launch template")
	}
	if get(t, asg.Properties, "LaunchTemplate", "LaunchTemplateId") == nil {
		t.Error("ASG doesn't reference the launch template directly")
	}
	if n := len(get(t, asg.Properties, "VPCZoneIdentifier").([]any)); n != 2 {
		t.Errorf("ASG spans %d subnets, want 2", n)
	}

	data := get(t, app.only(t, "AWS::EC2::LaunchTemplate").Properties, "LaunchTemplateData").(map[string]any)
	if got := str(data["InstanceType"]); got != "t4g.micro" {
		t.Errorf("InstanceType = %s, want t4g.micro", got)
	}
	if _, ok := data["InstanceMarketOptions"]; ok {
		t.Error("launch template requests a Spot market, want On-Demand")
	}
	ami := app.Parameters["ImageId"]
	if ami["Type"] != "AWS::SSM::Parameter::Value<AWS::EC2::Image::Id>" || ami["Default"] != "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64" {
		t.Errorf("ImageId parameter = %v, want the latest AL2023 arm64 AMI via SSM", ami)
	}
	ebs := get(t, data, "BlockDeviceMappings", 0, "Ebs")
	if str(get(t, ebs, "VolumeSize")) != "8" || str(get(t, ebs, "VolumeType")) != "gp3" {
		t.Errorf("root volume = %v, want 8 GiB gp3", ebs)
	}
}

// TestDeploysToUsEast2 ensures CI deploys and lints in us-east-2, the only
// region the organization's SCP allows.
func TestDeploysToUsEast2(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Env   map[string]string `yaml:"env"`
			Steps []struct {
				Name string            `yaml:"name"`
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatal(err)
	}
	deploy := wf.Jobs["deploy"]
	if got := deploy.Env["AWS_REGION"]; got != "us-east-2" {
		t.Errorf("deploy AWS_REGION = %q, want us-east-2", got)
	}
	for _, s := range deploy.Steps {
		if strings.HasPrefix(s.Uses, "aws-actions/configure-aws-credentials") && s.With["aws-region"] != "us-east-2" {
			t.Errorf("configure-aws-credentials aws-region = %q, want us-east-2", s.With["aws-region"])
		}
	}
	for _, s := range wf.Jobs["test"].Steps {
		if strings.Contains(s.Run, "cfn-lint") && !strings.Contains(s.Run, "--regions us-east-2") {
			t.Errorf("cfn-lint step %q doesn't lint for us-east-2", s.Run)
		}
	}
	if strings.Contains(string(b), "us-east-1") {
		t.Error("ci.yml still mentions us-east-1")
	}
}

// TestAppAllowsNoInboundTraffic ensures nothing can connect to the instance:
// no ingress rules anywhere, no NAT gateway (cost), no SSH key (access is SSM
// Session Manager only), and IMDSv2 is required.
func TestAppAllowsNoInboundTraffic(t *testing.T) {
	app := load(t, "app.yml")
	sg := app.only(t, "AWS::EC2::SecurityGroup")
	if _, ok := sg.Properties["SecurityGroupIngress"]; ok {
		t.Error("security group has ingress rules, want none")
	}
	for _, typ := range []string{"AWS::EC2::SecurityGroupIngress", "AWS::EC2::NatGateway"} {
		if n := len(app.ofType(typ)); n != 0 {
			t.Errorf("template has %d %s, want 0", n, typ)
		}
	}
	data := get(t, app.only(t, "AWS::EC2::LaunchTemplate").Properties, "LaunchTemplateData").(map[string]any)
	if _, ok := data["KeyName"]; ok {
		t.Error("launch template sets an SSH KeyName, want none")
	}
	if got := str(get(t, data, "MetadataOptions", "HttpTokens")); got != "required" {
		t.Errorf("HttpTokens = %s, want required (IMDSv2)", got)
	}
	var policies []string
	for _, p := range get(t, app.only(t, "AWS::IAM::Role").Properties, "ManagedPolicyArns").([]any) {
		policies = append(policies, str(p))
	}
	if !slices.Contains(policies, "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore") {
		t.Errorf("instance role policies = %v, want AmazonSSMManagedInstanceCore for Session Manager", policies)
	}
}

// TestAppStorageLimits keeps storage within the small free allowances: ECR
// keeps only the last 5 images and logs are kept 14 days.
func TestAppStorageLimits(t *testing.T) {
	app := load(t, "app.yml")
	var policy struct {
		Rules []struct {
			Selection struct {
				CountType   string `json:"countType"`
				CountNumber int    `json:"countNumber"`
			} `json:"selection"`
		} `json:"rules"`
	}
	text := str(get(t, app.only(t, "AWS::ECR::Repository").Properties, "LifecyclePolicy", "LifecyclePolicyText"))
	if err := json.Unmarshal([]byte(text), &policy); err != nil {
		t.Fatalf("lifecycle policy: %v", err)
	}
	if len(policy.Rules) != 1 || policy.Rules[0].Selection.CountType != "imageCountMoreThan" || policy.Rules[0].Selection.CountNumber != 5 {
		t.Errorf("lifecycle policy = %s, want one rule keeping 5 images", text)
	}
	if got := str(app.only(t, "AWS::Logs::LogGroup").Properties["RetentionInDays"]); got != "14" {
		t.Errorf("log retention = %s days, want 14", got)
	}
}

// TestAppNamesMatchStartScript ensures the fixed resource names the instance
// script relies on (secrets, image tag parameter, ECR repository, log group)
// are exactly the ones the template creates, and that the image tag starts as
// "none" until CI deploys the first image.
func TestAppNamesMatchStartScript(t *testing.T) {
	app := load(t, "app.yml")
	script := userData(t, app)

	names := map[string]string{}
	for _, s := range app.ofType("AWS::SecretsManager::Secret") {
		names[str(s.Properties["Name"])] = "secret"
	}
	param := app.only(t, "AWS::SSM::Parameter")
	names[str(param.Properties["Name"])] = "parameter"
	names[str(app.only(t, "AWS::ECR::Repository").Properties["RepositoryName"])] = "repository"
	names[str(app.only(t, "AWS::Logs::LogGroup").Properties["LogGroupName"])] = "log group"

	for _, want := range []string{"destiny-scheduler/discord-token", "destiny-scheduler/database-url", "/destiny-scheduler/image-tag", "destiny-scheduler", "/destiny-scheduler/bot"} {
		if names[want] == "" {
			t.Errorf("template doesn't create %q (has %v)", want, names)
		}
		if !strings.Contains(script, want) {
			t.Errorf("start script doesn't reference %q", want)
		}
	}
	if got := str(param.Properties["Value"]); got != "none" {
		t.Errorf("image tag parameter starts as %q, want none", got)
	}
}

// heredoc returns the body of the first `<<'MARK'` heredoc in script.
func heredoc(t *testing.T, script, mark string) string {
	t.Helper()
	_, rest, ok := strings.Cut(script, "<<'"+mark+"'\n")
	if !ok {
		t.Fatalf("script has no <<'%s' heredoc", mark)
	}
	body, _, ok := strings.Cut(rest, "\n"+mark+"\n")
	if !ok {
		t.Fatalf("heredoc %s is not terminated", mark)
	}
	return body + "\n"
}

// TestInstanceScriptsPassShellcheck lints the instance user data and the
// destiny-bot-start script it installs. The start script is linted on its own
// because shellcheck treats quoted heredoc contents as plain text. It uses a
// local shellcheck or the koalaman/shellcheck Docker image; CI must run it,
// elsewhere it's skipped if neither is available.
func TestInstanceScriptsPassShellcheck(t *testing.T) {
	userdata := userData(t, load(t, "app.yml"))
	scripts := map[string]string{
		"user-data.sh":         userdata,
		"destiny-bot-start.sh": heredoc(t, userdata, "SCRIPT"),
	}
	dir := t.TempDir()
	var files []string
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	slices.Sort(files)
	args := append([]string{"--severity=style"}, files...)

	var cmd *exec.Cmd
	if path, err := exec.LookPath("shellcheck"); err == nil {
		cmd = exec.Command(path, args...)
		cmd.Dir = dir
	} else if path, err := exec.LookPath("docker"); err == nil {
		cmd = exec.Command(path, append([]string{"run", "--rm", "-v", dir + ":/mnt", "koalaman/shellcheck:stable"}, args...)...)
	} else if os.Getenv("CI") != "" {
		t.Fatal("shellcheck is required in CI")
	} else {
		t.Skip("neither shellcheck nor docker is available")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("shellcheck found problems:\n%s", out)
	}
}

// TestBootstrapTrustsOnlyThisRepo ensures only GitHub Actions jobs of this
// repository running in the PRD environment can assume the deploy role.
func TestBootstrapTrustsOnlyThisRepo(t *testing.T) {
	boot := load(t, "bootstrap.yml")
	oidc := boot.only(t, "AWS::IAM::OIDCProvider")
	if oidc.Properties["Url"] != "https://token.actions.githubusercontent.com" {
		t.Errorf("OIDC provider Url = %v", oidc.Properties["Url"])
	}

	deploy, ok := boot.Resources["DeployRole"]
	if !ok {
		t.Fatal("bootstrap has no DeployRole")
	}
	stmt := get(t, deploy.Properties, "AssumeRolePolicyDocument", "Statement", 0)
	if str(get(t, stmt, "Action")) != "sts:AssumeRoleWithWebIdentity" {
		t.Errorf("trust action = %v", get(t, stmt, "Action"))
	}
	cond := get(t, stmt, "Condition").(map[string]any)
	if len(cond) != 1 || cond["StringEquals"] == nil {
		t.Fatalf("trust condition = %v, want only exact StringEquals (no wildcards)", cond)
	}
	eq := cond["StringEquals"].(map[string]any)
	if eq["token.actions.githubusercontent.com:aud"] != "sts.amazonaws.com" ||
		eq["token.actions.githubusercontent.com:sub"] != "repo:thonycsdev/DestinyScheduler:environment:PRD" {
		t.Errorf("trust StringEquals = %v, want aud sts.amazonaws.com and sub for this repo's PRD environment", eq)
	}
}

// TestBootstrapDeployRoleIsScoped ensures the CI role holds no wildcard
// actions and can only pass the CloudFormation execution role.
func TestBootstrapDeployRoleIsScoped(t *testing.T) {
	boot := load(t, "bootstrap.yml")
	deploy := boot.Resources["DeployRole"]
	for _, p := range get(t, deploy.Properties, "Policies").([]any) {
		for _, s := range get(t, p, "PolicyDocument", "Statement").([]any) {
			actions := get(t, s, "Action")
			list, ok := actions.([]any)
			if !ok {
				list = []any{actions}
			}
			for _, a := range list {
				if strings.HasSuffix(str(a), "*") {
					t.Errorf("deploy role allows wildcard action %q", a)
				}
				if a == "iam:PassRole" {
					if want := map[string]any{"Fn::GetAtt": []any{"CloudFormationExecutionRole", "Arn"}}; fmt.Sprint(get(t, s, "Resource")) != fmt.Sprint(want) {
						t.Errorf("iam:PassRole on %v, want only the CloudFormation execution role", get(t, s, "Resource"))
					}
				}
			}
		}
	}
}
