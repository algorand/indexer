package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

// MakeMockDB initializes a sql.DB object with a mocked driver loaded
// to respond with the provided statements.
func MakeMockDB(statements []*MockStmt) *sql.DB {
	driver := &MockDriver{
		statementIdx: 0,
		Statements:   statements,
	}
	return sql.OpenDB(driver)
}

// MockDriver is the core of a mocked sql.DB object. It implements
// several interfaces: driver.Connector, driver.Conn,
type MockDriver struct {
	statementIdx int
	Statements   []*MockStmt
}

// Connect - part of driver.Connector
func (d *MockDriver) Connect(ctx context.Context) (driver.Conn, error) {
	return d.Open("")
}

// Driver - part of driver.Connector
func (d *MockDriver) Driver() driver.Driver {
	return d
}

// Open - Part of sql.Driver
func (d *MockDriver) Open(name string) (driver.Conn, error) {
	//fmt.Println("driver.Driver : Open")
	return d, nil
}

// Prepare - Part of driver.Conn
func (d *MockDriver) Prepare(query string) (driver.Stmt, error) {
	d.statementIdx++

	if d.statementIdx > len(d.Statements) {
		return nil, errors.New("not enough statements loaded into mock driver")
	}

	numParts := 0
	for _, v := range query {
		if string(v) == "$" {
			numParts++
		}
	}

	if numParts != d.Statements[d.statementIdx-1].parts {
		return nil, fmt.Errorf("unexpected number of arguments in statement: %s", query)
	}

	return d.Statements[d.statementIdx-1], nil
}

// Close - Part of driver.Conn
func (d *MockDriver) Close() error {
	//fmt.Println("driver.Conn : Close")
	return nil
}

// Begin - Part of driver.Conn
func (d *MockDriver) Begin() (driver.Tx, error) {
	//fmt.Println("driver.Conn : Begin")
	return &MockTx{}, nil
}

// MakeMockStmt is used to orchestrate the lifecycle of a sql.DB statement.
func MakeMockStmt(parts int, columns []string, rows [][]interface{}) *MockStmt {
	return &MockStmt{
		parts:   parts,
		columns: columns,
		rowNum:  0,
		rows:    rows,
	}
}

// MockStmt orchestrates the lifecycle of a sql.DB statement
// It implements driver.Stmt and driver.Rows. If I needed them,
// it probably would have also implemented driver.Tx and
// driver.Result.
type MockStmt struct {
	// number of parameterized arguments
	parts int

	columns []string
	rowNum  int
	rows    [][]interface{}
}

// Close - Part of driver.Stmt interfaces
func (s *MockStmt) Close() error {
	//fmt.Println("driver.Stmt / driver.Rows - Close")
	return nil
}

// NumInput - Part of driver.Stmt
func (s *MockStmt) NumInput() int {
	//fmt.Println("driver.Stmt - NumInput")
	return s.parts
}

// Exec - Part of driver.Stmt
func (s *MockStmt) Exec(args []driver.Value) (driver.Result, error) {
	//fmt.Println("driver.Stmt - Exec")
	return &MockResult{
		insertId: 0,
		rows:     0,
	}, nil
}

// Query - Part of driver.Stmt
func (s *MockStmt) Query(args []driver.Value) (driver.Rows, error) {
	//fmt.Println("driver.Stmt - Query")
	return s, nil
}

// Columns - Part of driver.Rows
func (s *MockStmt) Columns() []string {
	//fmt.Println("driver.Rows - Columns")
	return s.columns
}

// Next - Part of driver.Rows
func (s *MockStmt) Next(dest []driver.Value) error {
	//fmt.Println("driver.Rows - Next")
	if s.rowNum > len(s.rows) {
		return errors.New("no more rows loaded in mock driver")
	}

	if len(dest) != len(s.rows[s.rowNum]) {
		return errors.New("unexpected number of destination rows in mock driver")
	}

	// Copy mock values into the destination
	for i, v := range s.rows[s.rowNum] {
		dest[i] = v
	}

	s.rowNum++
	return nil
}

// MockResult is something I didn't need but is part of the interface.
type MockResult struct {
	insertId int64
	rows     int64
}

// LastInsertId - Part of driver.Result interface
func (s *MockResult) LastInsertId() (int64, error) {
	//fmt.Println("driver.Result - LastInsertId")
	return s.insertId, nil
}

// RowsAffected - Part of driver.Result interface
func (s *MockResult) RowsAffected() (int64, error) {
	//fmt.Println("driver.Result - RowsAffected")
	return s.rows, nil
}

// MockTx is something I didn't need but is part of the interface.
type MockTx struct {
}

func (t *MockTx) Commit() error {
	//fmt.Println("driver.Tx - Commit")
	return nil
}

func (t *MockTx) Rollback() error {
	//fmt.Println("driver.Tx - Rollback")
	return nil
}
