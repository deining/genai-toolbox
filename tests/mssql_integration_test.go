//go:build integration && mssql

// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	MSSQL_SOURCE_KIND = "mssql"
	MSSQL_TOOL_KIND   = "mssql-sql"
	MSSQL_DATABASE    = os.Getenv("MSSQL_DATABASE")
	MSSQL_HOST        = os.Getenv("MSSQL_HOST")
	MSSQL_PORT        = os.Getenv("MSSQL_PORT")
	MSSQL_USER        = os.Getenv("MSSQL_USER")
	MSSQL_PASS        = os.Getenv("MSSQL_PASS")
)

func getMsSQLVars(t *testing.T) map[string]any {
	switch "" {
	case MSSQL_DATABASE:
		t.Fatal("'MSSQL_DATABASE' not set")
	case MSSQL_HOST:
		t.Fatal("'MSSQL_HOST' not set")
	case MSSQL_PORT:
		t.Fatal("'MSSQL_PORT' not set")
	case MSSQL_USER:
		t.Fatal("'MSSQL_USER' not set")
	case MSSQL_PASS:
		t.Fatal("'MSSQL_PASS' not set")
	}

	return map[string]any{
		"kind":     MSSQL_SOURCE_KIND,
		"host":     MSSQL_HOST,
		"port":     MSSQL_PORT,
		"database": MSSQL_DATABASE,
		"user":     MSSQL_USER,
		"password": MSSQL_PASS,
	}
}

// Copied over from mssql.go
func initMssqlConnection(host, port, user, pass, dbname string) (*sql.DB, error) {
	// Create dsn
	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, pass, host, port, dbname)

	// Open database connection
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return db, nil
}

func TestMsSQLToolEndpoints(t *testing.T) {
	sourceConfig := getMsSQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var args []string

	pool, err := initMssqlConnection(MSSQL_HOST, MSSQL_PORT, MSSQL_USER, MSSQL_PASS, MSSQL_DATABASE)
	if err != nil {
		t.Fatalf("unable to create SQL Server connection pool: %s", err)
	}

	// create table name with UUID
	tableNameParam := "param_table_" + strings.Replace(uuid.New().String(), "-", "", -1)
	tableNameAuth := "auth_table_" + strings.Replace(uuid.New().String(), "-", "", -1)

	// set up data for param tool
	create_statement1, insert_statement1, tool_statement1, params1 := GetMssqlParamToolInfo(tableNameParam)
	teardownTable1 := SetupMsSQLTable(t, ctx, pool, create_statement1, insert_statement1, tableNameParam, params1)
	defer teardownTable1(t)

	// set up data for auth tool
	create_statement2, insert_statement2, tool_statement2, params2 := GetMssqlLAuthToolInfo(tableNameAuth)
	teardownTable2 := SetupMsSQLTable(t, ctx, pool, create_statement2, insert_statement2, tableNameAuth, params2)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := GetToolsConfig(sourceConfig, MSSQL_TOOL_KIND, tool_statement1, tool_statement2)

	cmd, cleanup, err := StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := cmd.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`))
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	RunToolGetTest(t)

	select_1_want := "[{\"\":1}]"
	fail_invocation_want := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"unable to execute query: mssql: Could not find stored procedure 'SELEC'."}],"isError":true}}`
	RunToolInvokeTest(t, select_1_want)
	RunMCPToolCallMethod(t, fail_invocation_want)
}
