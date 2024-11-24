package database

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/encryption"
	"github.com/gocql/gocql"
)

type Cassandra struct {
	Db *gocql.Session
}

func NewCassandra() *Cassandra {
	// Connect to Cassandra without specifying a keyspace for schema creation
	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Consistency = gocql.Quorum

	// Create a temporary session
	tempSession, err := cluster.CreateSession()
	if err != nil {
		log.Fatalf("Failed to connect to Cassandra: %v", err)
	}
	defer tempSession.Close()

	// Create the keyspace and schema
	err = createSchema(tempSession)
	if err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	// Connect to the xenomorph keyspace
	cluster.Keyspace = "xenomorph"
	session, err := cluster.CreateSession()
	if err != nil {
		log.Fatalf("Failed to connect to Cassandra: %v", err)
	}

	// Return the Cassandra instance
	return &Cassandra{
		Db: session,
	}
}

func createKeyspace(session *gocql.Session) error {
	query := `
	CREATE KEYSPACE IF NOT EXISTS xenomorph
	WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};
	`
	return session.Query(query).Exec()
}

func createSchema(session *gocql.Session) error {
	err := createKeyspace(session)
	if err != nil {
		return fmt.Errorf("failed to create keyspace: %v", err)
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS xenomorph.clients (
        id UUID PRIMARY KEY,
        public_key TEXT,
        private_key TEXT,
        data TEXT
    );`,
		`CREATE TABLE IF NOT EXISTS xenomorph.files (
        bucket_id TEXT PRIMARY KEY,
        client_id UUID,
        file_name TEXT,
        file_extension TEXT,
        file_size INT,
        created_at TIMESTAMP,
        updated_at TIMESTAMP
    );`,
	}

	for _, query := range queries {
		err := session.Query(query).Exec()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Cassandra) Close() {
	c.Db.Close()
}

func (c *Cassandra) RegisterClient(clientUUID string) (publicKey string, err error) {
	if exists, err := c.ClientExists(clientUUID); err != nil {
		return "", err
	} else if exists {
		return "", nil
	}

	publicKey, privateKey, err := encryption.GenerateRSAKeys()
	if err != nil {
		return "", fmt.Errorf("failed to generate RSA keys: %v", err)
	}

	query := c.Db.Query(`INSERT INTO xenomorph.clients (id, public_key, private_key) VALUES (?, ?, ?)`,
		clientUUID, publicKey, privateKey)
	err = query.Exec()
	if err != nil {
		return "", err
	}

	return publicKey, nil
}

func (c *Cassandra) InsertFile(uuid string, file common.FileData) error {
	query := c.Db.Query(`INSERT INTO xenomorph.files (bucket_id, client_id, file_name, file_extension, file_size, created_at, updated_at) VALUES (?, ?, ?, ?, ?, toTimestamp(now()), toTimestamp(now()))`,
		file.BucketID, uuid, file.FileName, file.FileExtension, file.FileSize)
	err := query.Exec()
	if err != nil {
		return err
	}

	return nil
}

func (c *Cassandra) UpdateClient(clientUUID string, data *common.ClientData) error {
	dataString, err := json.Marshal(data)
	if err != nil {
		return err
	}

	query := c.Db.Query(`UPDATE xenomorph.clients SET data = ? WHERE id = ?`, dataString, clientUUID)
	err = query.Exec()
	if err != nil {
		return err
	}

	return nil
}

func (c *Cassandra) GetClient(clientUUID string) (common.ClientData, error) {
	var dataString string
	query := c.Db.Query(`SELECT data FROM xenomorph.clients WHERE id = ?`, clientUUID)
	err := query.Scan(&dataString)
	if err != nil {
		return common.ClientData{}, err
	}

	var data common.ClientData
	err = json.Unmarshal([]byte(dataString), &data)
	if err != nil {
		return common.ClientData{}, err
	}

	return data, nil
}

func (c *Cassandra) ClientExists(clientUUID string) (bool, error) {
	var count int
	query := c.Db.Query(`SELECT COUNT(*) FROM xenomorph.clients WHERE id = ?`, clientUUID)
	err := query.Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (c *Cassandra) GetClientEssentials(clientUUID string) (string, error) {
	var publicKey string
	query := c.Db.Query(`SELECT private_key FROM xenomorph.clients WHERE id = ?`, clientUUID)
	err := query.Scan(&publicKey)
	if err != nil {
		return "", err
	}

	return publicKey, nil
}
