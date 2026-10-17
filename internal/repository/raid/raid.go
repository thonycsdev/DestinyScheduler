// Package raid saves and loads raids in Postgres, using internal/database's
// pool and schema. The fireteam rules themselves live in internal/raid; this
// package only applies them under a row lock so concurrent changes are safe.
package raid

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/anthonycoutinho/destinyscheduler/internal/raid"
)

var ErrNotFound = errors.New("raid not found")

// Repository stores raids and their fireteams.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// selectRaids loads raids with their members in join order. Callers append a
// WHERE clause on r and GROUP BY/ORDER BY as needed.
const selectRaids = `
	SELECT r.id::text, r.name, r.starts_at, r.leader_id,
	       coalesce(r.channel_id, ''), coalesce(r.message_id, ''),
	       coalesce(array_agg(m.user_id ORDER BY m.joined_at) FILTER (WHERE m.user_id IS NOT NULL), '{}')
	FROM raids r
	LEFT JOIN raid_members m ON m.raid_id = r.id`

func scanRaid(row pgx.Row) (domain.Raid, error) {
	var r domain.Raid
	err := row.Scan(&r.ID, &r.Name, &r.StartsAt, &r.LeaderID, &r.ChannelID, &r.MessageID, &r.Members)
	r.StartsAt = r.StartsAt.UTC()
	return r, err
}

// notFound maps "no row" and malformed IDs (e.g. from a stale or tampered
// Discord button) to ErrNotFound.
func notFound(err error) error {
	var pgErr *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || errors.As(err, &pgErr) && pgErr.Code == "22P02" {
		return ErrNotFound
	}
	return err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func get(ctx context.Context, q querier, id string) (domain.Raid, error) {
	r, err := scanRaid(q.QueryRow(ctx, selectRaids+" WHERE r.id = $1 GROUP BY r.id", id))
	if err != nil {
		return domain.Raid{}, notFound(err)
	}
	return r, nil
}

// Create stores a scheduled raid and its members, and returns it with the
// ID assigned by the database.
func (repo *Repository) Create(ctx context.Context, r domain.Raid) (domain.Raid, error) {
	err := pgx.BeginFunc(ctx, repo.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO raids (name, starts_at, leader_id, channel_id, message_id)
			VALUES ($1, $2, $3, nullif($4, ''), nullif($5, ''))
			RETURNING id::text`,
			r.Name, r.StartsAt, r.LeaderID, r.ChannelID, r.MessageID).Scan(&r.ID)
		if err != nil {
			return err
		}
		// One insert per member so joined_at (clock_timestamp) keeps their order.
		for _, m := range r.Members {
			if _, err := tx.Exec(ctx, "INSERT INTO raid_members (raid_id, user_id) VALUES ($1, $2)", r.ID, m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.Raid{}, fmt.Errorf("creating raid: %w", err)
	}
	return r, nil
}

// Get loads a raid with its members in join order.
func (repo *Repository) Get(ctx context.Context, id string) (domain.Raid, error) {
	return get(ctx, repo.pool, id)
}

// update locks the raid row, loads it, lets change apply a domain rule, then
// saves with persist. The lock serializes concurrent joins and leaves.
func (repo *Repository) update(ctx context.Context, id string, change func(*domain.Raid) error, persist func(pgx.Tx) error) (domain.Raid, error) {
	var r domain.Raid
	err := pgx.BeginFunc(ctx, repo.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM raids WHERE id = $1 FOR UPDATE", id); err != nil {
			return notFound(err)
		}
		var err error
		if r, err = get(ctx, tx, id); err != nil {
			return err
		}
		if err := change(&r); err != nil {
			return err
		}
		return persist(tx)
	})
	if err != nil {
		return domain.Raid{}, err
	}
	return r, nil
}

// Join adds userID to the raid's fireteam, applying domain.Raid.Join's rules
// at time now.
func (repo *Repository) Join(ctx context.Context, id, userID string, now time.Time) (domain.Raid, error) {
	return repo.update(ctx, id,
		func(r *domain.Raid) error { return r.Join(userID, now) },
		func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "INSERT INTO raid_members (raid_id, user_id) VALUES ($1, $2)", id, userID)
			return err
		})
}

// Leave removes userID from the raid's fireteam, applying domain.Raid.Leave's
// rules at time now.
func (repo *Repository) Leave(ctx context.Context, id, userID string, now time.Time) (domain.Raid, error) {
	return repo.update(ctx, id,
		func(r *domain.Raid) error { return r.Leave(userID, now) },
		func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM raid_members WHERE raid_id = $1 AND user_id = $2", id, userID)
			return err
		})
}

// SetMessage records where the raid's Discord message was posted.
func (repo *Repository) SetMessage(ctx context.Context, id, channelID, messageID string) error {
	tag, err := repo.pool.Exec(ctx, "UPDATE raids SET channel_id = $2, message_id = $3 WHERE id = $1", id, channelID, messageID)
	if err != nil {
		return notFound(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete cancels a raid; its members are removed with it.
func (repo *Repository) Delete(ctx context.Context, id string) error {
	tag, err := repo.pool.Exec(ctx, "DELETE FROM raids WHERE id = $1", id)
	if err != nil {
		return notFound(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUpcoming returns raids starting after now, soonest first.
func (repo *Repository) ListUpcoming(ctx context.Context, now time.Time) ([]domain.Raid, error) {
	rows, err := repo.pool.Query(ctx, selectRaids+" WHERE r.starts_at > $1 GROUP BY r.id ORDER BY r.starts_at", now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var raids []domain.Raid
	for rows.Next() {
		r, err := scanRaid(rows)
		if err != nil {
			return nil, err
		}
		raids = append(raids, r)
	}
	return raids, rows.Err()
}
