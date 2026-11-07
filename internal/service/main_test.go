package service

import (
	"os"
	"testing"

	"nexmap.net/internal/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }
