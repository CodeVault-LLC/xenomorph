package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

type Clickhouse struct {
    Conn clickhouse.Conn
}

func NewClickhouse() *Clickhouse {
    opts := &clickhouse.Options{
        Addr: []string{"127.0.0.1:9001"},
        Auth: clickhouse.Auth{Database: "default", Username: "default", Password: ""},
        Settings: clickhouse.Settings{
            "max_execution_time": 60,
        },
    }

    conn, err := clickhouse.Open(opts)
    if err != nil {
        logger.L().Error("Failed to connect to ClickHouse:", zap.Error(err))
        return nil
    }

    if err := createSchema(context.Background(), conn); err != nil {
        logger.L().Error("Failed to create schema:", zap.Error(err))
        return nil
    }

		logger.L().Info("Connected to ClickHouse successfully")
		return &Clickhouse{
			Conn: conn,
		}
	}

func createSchema(ctx context.Context, conn clickhouse.Conn) error {
    ddl := []string{
        `CREATE DATABASE IF NOT EXISTS smat`,
        `CREATE TABLE IF NOT EXISTS smat.clients (
            id UUID,
            data JSON,
            created_at DateTime
            PRIMARY KEY id
        ) ENGINE = MergeTree()
          ORDER BY id`,
        `CREATE TABLE IF NOT EXISTS smat.files (
            bucket_id String,
            client_id UUID,
            file_name String,
            file_extension String,
            file_size UInt64,
            created_at DateTime,
            updated_at DateTime
        ) ENGINE = MergeTree()
          ORDER BY bucket_id`,
    }

    for _, q := range ddl {
        if err := conn.Exec(ctx, q); err != nil {
            return fmt.Errorf("schema exec failed: %w", err)
        }
    }
    return nil
}

func (c *Clickhouse) Close() error {
    return c.Conn.Close()
}

// RegisterClient registers a new client in the ClickHouse database.
func (c *Clickhouse) RegisterClient(ctx context.Context, uuid string) error {
    exists, err := c.ClientExists(ctx, uuid)
    if err != nil {
        return err
    }
    if exists {
        return nil
    }

    query := `
        INSERT INTO smat.clients (id, created_at)
        VALUES (?, ?)
    `
    if err := c.Conn.Exec(ctx, query, uuid, time.Now()); err != nil {
        return err
    }

    logger.L().Info("Registered new client in ClickHouse", zap.String("uuid", uuid))
    return nil
}

/*
// GetClientEssentials retrieves essential client information from ClickHouse.
func (c *Clickhouse) InsertFile(ctx context.Context, uuid string, file common.FileData) error {
    query := `
        INSERT INTO smat.files 
        (bucket_id, client_id, file_name, file_extension, file_size, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `
    now := time.Now()
    return c.Conn.Exec(ctx, query, file.BucketID, uuid, file.FileName, file.FileExtension, file.FileSize, now, now)
}*/

func (c *Clickhouse) UpdateClient(ctx context.Context, uuid string, data *types.RegistrationData) error {
    dataString, err := json.Marshal(data)
    if err != nil {
        return err
    }
    query := `
        ALTER TABLE smat.clients
        UPDATE data = ? WHERE id = ?
    `
    return c.Conn.Exec(ctx, query, string(dataString), uuid)
}

func (c *Clickhouse) GetClient(ctx context.Context, uuid string) (types.RegistrationData, error) {
    var dataString string
    query := `SELECT data FROM smat.clients WHERE id = ? LIMIT 1`
    if err := c.Conn.QueryRow(ctx, query, uuid).Scan(&dataString); err != nil {
        return types.RegistrationData{}, err
    }

    var data types.RegistrationData
    if err := json.Unmarshal([]byte(dataString), &data); err != nil {
        return types.RegistrationData{}, err
    }
    return data, nil
}

func (c *Clickhouse) ClientExists(ctx context.Context, uuid string) (bool, error) {
    var count uint64
    query := `SELECT count() FROM smat.clients WHERE id = ?`
    if err := c.Conn.QueryRow(ctx, query, uuid).Scan(&count); err != nil {
        return false, err
    }
    return count > 0, nil
}
