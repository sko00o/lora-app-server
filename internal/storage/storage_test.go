package storage

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/brocaar/lora-app-server/internal/test"
)

type StorageTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase
}

func TestStorage(t *testing.T) {
	suite.Run(t, new(StorageTestSuite))
}
