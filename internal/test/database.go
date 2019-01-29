package test

import (
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/lora-app-server/internal/common"
	"github.com/brocaar/lora-app-server/internal/config"
)

// DatabaseTestSuiteBase provides the setup and teardown of the database
// for every test-run.
type DatabaseTestSuiteBase struct {
	db *common.DBLogger
	tx *common.TxLogger
	p  *redis.Pool
}

// SetupSuite is called once before starting the test-suite.
func (b *DatabaseTestSuiteBase) SetupSuite() {
	conf := GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		panic(err)
	}
	b.db = db
	MustResetDB(db)
	b.p = common.NewRedisPool(conf.RedisURL, 10, 0)

	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = b.p
}

// SetupTest is called before every test.
func (b *DatabaseTestSuiteBase) SetupTest() {
	tx, err := b.db.Beginx()
	if err != nil {
		panic(err)
	}
	b.tx = tx

	MustFlushRedis(b.p)
}

// TearDownTest is called after every test.
func (b *DatabaseTestSuiteBase) TearDownTest() {
	if err := b.tx.Rollback(); err != nil {
		panic(err)
	}
}

// TearDownSuite is called once after completing the tests.
func (b *DatabaseTestSuiteBase) TearDownSuite() {
	if err := b.db.Close(); err != nil {
		panic(err)
	}
	if err := b.p.Close(); err != nil {
		panic(err)
	}
}

// Tx returns a database transaction (which is rolled back after every
// test).
func (b *DatabaseTestSuiteBase) Tx() sqlx.Ext {
	return b.tx
}

// DB returns the database object.
func (b *DatabaseTestSuiteBase) DB() *common.DBLogger {
	return b.db
}

// RedisPool returns the redis.Pool object.
func (b *DatabaseTestSuiteBase) RedisPool() *redis.Pool {
	return b.p
}
