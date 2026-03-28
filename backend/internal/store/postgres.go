package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) EnsureTailnet(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tailnets (name)
		VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("ensure tailnet: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) ListServers(ctx context.Context, tailnetID int64, tags []string) ([]Server, error) {
	query := `
		SELECT
			s.id,
			s.name,
			host(s.tailscale_ip) AS tailscale_ip,
			s.ssh_user,
			s.created_at,
			s.updated_at,
			s.created_by,
			s.updated_by,
			COALESCE(array_remove(array_agg(DISTINCT t.value), NULL), '{}') AS tags
		FROM servers s
		LEFT JOIN server_tags st ON st.server_id = s.id
		LEFT JOIN tags t ON t.id = st.tag_id
		WHERE s.tailnet_id = $1
		  AND (
			COALESCE(array_length($2::text[], 1), 0) = 0
			OR s.id IN (
				SELECT stf.server_id
				FROM server_tags stf
				JOIN tags tf ON tf.id = stf.tag_id
				WHERE tf.tailnet_id = $1
				  AND tf.value = ANY($2::text[])
				GROUP BY stf.server_id
				HAVING COUNT(DISTINCT tf.value) = COALESCE(array_length($2::text[], 1), 0)
			)
		  )
		GROUP BY s.id
		ORDER BY s.name ASC, s.id ASC
	`

	rows, err := s.pool.Query(ctx, query, tailnetID, tags)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()

	return scanServers(rows)
}

func (s *PostgresStore) SearchServers(ctx context.Context, tailnetID int64, query string, tags []string, limit int) ([]Server, error) {
	if limit <= 0 {
		limit = 50
	}

	pattern := "%" + query + "%"

	rows, err := s.pool.Query(ctx, `
		SELECT
			s.id,
			s.name,
			host(s.tailscale_ip) AS tailscale_ip,
			s.ssh_user,
			s.created_at,
			s.updated_at,
			s.created_by,
			s.updated_by,
			COALESCE(array_remove(array_agg(DISTINCT t.value), NULL), '{}') AS tags
		FROM servers s
		LEFT JOIN server_tags st ON st.server_id = s.id
		LEFT JOIN tags t ON t.id = st.tag_id
		WHERE s.tailnet_id = $1
		  AND (
			s.name ILIKE $2
			OR s.ssh_user ILIKE $2
			OR host(s.tailscale_ip) ILIKE $2
			OR EXISTS (
				SELECT 1
				FROM server_tags sts
				JOIN tags ts ON ts.id = sts.tag_id
				WHERE sts.server_id = s.id
				  AND ts.value ILIKE $2
			)
		  )
		  AND (
			COALESCE(array_length($3::text[], 1), 0) = 0
			OR s.id IN (
				SELECT stf.server_id
				FROM server_tags stf
				JOIN tags tf ON tf.id = stf.tag_id
				WHERE tf.tailnet_id = $1
				  AND tf.value = ANY($3::text[])
				GROUP BY stf.server_id
				HAVING COUNT(DISTINCT tf.value) = COALESCE(array_length($3::text[], 1), 0)
			)
		  )
		GROUP BY s.id
		ORDER BY s.name ASC, s.id ASC
		LIMIT $4
	`, tailnetID, pattern, tags, limit)
	if err != nil {
		return nil, fmt.Errorf("search servers: %w", err)
	}
	defer rows.Close()

	return scanServers(rows)
}

func (s *PostgresStore) CreateServer(ctx context.Context, tailnetID int64, actor string, input CreateServerInput) (Server, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Server{}, fmt.Errorf("begin create server: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO servers (
			tailnet_id,
			name,
			tailscale_ip,
			ssh_user,
			created_by,
			updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id
	`, tailnetID, input.Name, input.TailscaleIP, input.SSHUser, actor).Scan(&id)
	if err != nil {
		return Server{}, mapWriteError("create server", err)
	}

	if err := replaceServerTags(ctx, tx, tailnetID, id, input.Tags); err != nil {
		return Server{}, err
	}

	server, err := getServerByID(ctx, tx, tailnetID, id)
	if err != nil {
		return Server{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Server{}, fmt.Errorf("commit create server: %w", err)
	}

	return server, nil
}

func (s *PostgresStore) UpdateServer(ctx context.Context, tailnetID int64, id int64, actor string, input UpdateServerInput) (Server, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Server{}, fmt.Errorf("begin update server: %w", err)
	}
	defer tx.Rollback(ctx)

	current, err := getServerByIDForUpdate(ctx, tx, tailnetID, id)
	if err != nil {
		return Server{}, err
	}

	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}

	ip := current.TailscaleIP
	if input.TailscaleIP != nil {
		ip = *input.TailscaleIP
	}

	sshUser := current.SSHUser
	if input.SSHUser != nil {
		sshUser = *input.SSHUser
	}

	if _, err := tx.Exec(ctx, `
		UPDATE servers
		SET name = $1,
			tailscale_ip = $2,
			ssh_user = $3,
			updated_at = NOW(),
			updated_by = $4
		WHERE id = $5
		  AND tailnet_id = $6
	`, name, ip, sshUser, actor, id, tailnetID); err != nil {
		return Server{}, mapWriteError("update server", err)
	}

	if input.Tags != nil {
		if err := replaceServerTags(ctx, tx, tailnetID, id, *input.Tags); err != nil {
			return Server{}, err
		}
	}

	server, err := getServerByID(ctx, tx, tailnetID, id)
	if err != nil {
		return Server{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Server{}, fmt.Errorf("commit update server: %w", err)
	}

	return server, nil
}

func (s *PostgresStore) DeleteServer(ctx context.Context, tailnetID int64, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM servers WHERE id = $1 AND tailnet_id = $2`, id, tailnetID)
	if err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type serverScanner interface {
	Scan(dest ...any) error
}

func scanServerRow(row serverScanner) (Server, error) {
	var server Server
	if err := row.Scan(
		&server.ID,
		&server.Name,
		&server.TailscaleIP,
		&server.SSHUser,
		&server.CreatedAt,
		&server.UpdatedAt,
		&server.CreatedBy,
		&server.UpdatedBy,
		&server.Tags,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Server{}, ErrNotFound
		}
		return Server{}, err
	}
	return server, nil
}

func scanServers(rows pgx.Rows) ([]Server, error) {
	servers := make([]Server, 0)
	for rows.Next() {
		server, err := scanServerRow(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return servers, nil
}

func getServerByID(ctx context.Context, querier queryer, tailnetID, id int64) (Server, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			s.id,
			s.name,
			host(s.tailscale_ip) AS tailscale_ip,
			s.ssh_user,
			s.created_at,
			s.updated_at,
			s.created_by,
			s.updated_by,
			COALESCE(array_remove(array_agg(DISTINCT t.value), NULL), '{}') AS tags
		FROM servers s
		LEFT JOIN server_tags st ON st.server_id = s.id
		LEFT JOIN tags t ON t.id = st.tag_id
		WHERE s.tailnet_id = $1
		  AND s.id = $2
		GROUP BY s.id
	`, tailnetID, id)
	server, err := scanServerRow(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Server{}, ErrNotFound
		}
		return Server{}, fmt.Errorf("get server: %w", err)
	}
	return server, nil
}

func getServerByIDForUpdate(ctx context.Context, tx pgx.Tx, tailnetID, id int64) (Server, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			id,
			name,
			host(tailscale_ip) AS tailscale_ip,
			ssh_user,
			created_at,
			updated_at,
			created_by,
			updated_by,
			ARRAY[]::text[] AS tags
		FROM servers
		WHERE tailnet_id = $1
		  AND id = $2
		FOR UPDATE
	`, tailnetID, id)
	server, err := scanServerRow(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Server{}, ErrNotFound
		}
		return Server{}, fmt.Errorf("lock server: %w", err)
	}
	return server, nil
}

func replaceServerTags(ctx context.Context, tx pgx.Tx, tailnetID, serverID int64, tags []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM server_tags WHERE server_id = $1`, serverID); err != nil {
		return fmt.Errorf("clear server tags: %w", err)
	}

	for _, tag := range tags {
		var tagID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO tags (tailnet_id, value)
			VALUES ($1, $2)
			ON CONFLICT (tailnet_id, value) DO UPDATE SET value = EXCLUDED.value
			RETURNING id
		`, tailnetID, tag).Scan(&tagID); err != nil {
			return mapWriteError("upsert tag", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO server_tags (server_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, serverID, tagID); err != nil {
			return fmt.Errorf("attach tag %q: %w", tag, err)
		}
	}

	return nil
}

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func mapWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}
