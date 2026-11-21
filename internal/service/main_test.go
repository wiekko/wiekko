// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"os"
	"testing"

	"wiekko.net/internal/database/dbtest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }
