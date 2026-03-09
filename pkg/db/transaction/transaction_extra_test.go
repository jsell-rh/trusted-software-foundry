package transaction

// transaction_extra_test.go covers:
//   Build (constructor)
//   MarkedForRollback (default false)
//   TxID
//   SetRollbackFlag
//   Commit (nil tx returns error)
//   Rollback (nil tx returns error)

import (
	"testing"
)

// --------------------------------------------------------------------------
// Build
// --------------------------------------------------------------------------

func TestBuild_NotNil(t *testing.T) {
	tx := Build(nil, 42, false)
	if tx == nil {
		t.Fatal("Build returned nil")
	}
}

// --------------------------------------------------------------------------
// MarkedForRollback (always defaults to false per defaultRollbackPolicy)
// --------------------------------------------------------------------------

func TestBuild_DefaultRollback(t *testing.T) {
	tx := Build(nil, 1, true) // flag param is ignored per impl
	if tx.MarkedForRollback() != false {
		t.Error("MarkedForRollback should default to false regardless of Build argument")
	}
}

// --------------------------------------------------------------------------
// TxID
// --------------------------------------------------------------------------

func TestTxID(t *testing.T) {
	tx := Build(nil, 99, false)
	if tx.TxID() != 99 {
		t.Errorf("TxID() = %d, want 99", tx.TxID())
	}
}

// --------------------------------------------------------------------------
// SetRollbackFlag
// --------------------------------------------------------------------------

func TestSetRollbackFlag_True(t *testing.T) {
	tx := Build(nil, 1, false)
	tx.SetRollbackFlag(true)
	if !tx.MarkedForRollback() {
		t.Error("MarkedForRollback should be true after SetRollbackFlag(true)")
	}
}

func TestSetRollbackFlag_False(t *testing.T) {
	tx := Build(nil, 1, false)
	tx.SetRollbackFlag(true)
	tx.SetRollbackFlag(false)
	if tx.MarkedForRollback() {
		t.Error("MarkedForRollback should be false after SetRollbackFlag(false)")
	}
}

// --------------------------------------------------------------------------
// Commit — nil tx returns error
// --------------------------------------------------------------------------

func TestCommit_NilTx_ReturnsError(t *testing.T) {
	tx := Build(nil, 1, false)
	err := tx.Commit()
	if err == nil {
		t.Error("Commit with nil tx should return error")
	}
}

// --------------------------------------------------------------------------
// Rollback — nil tx returns error
// --------------------------------------------------------------------------

func TestRollback_NilTx_ReturnsError(t *testing.T) {
	tx := Build(nil, 1, false)
	err := tx.Rollback()
	if err == nil {
		t.Error("Rollback with nil tx should return error")
	}
}

// --------------------------------------------------------------------------
// Tx()
// --------------------------------------------------------------------------

func TestTx_NilSqlTx(t *testing.T) {
	tx := Build(nil, 1, false)
	if tx.Tx() != nil {
		t.Error("Tx() should return nil when no sql.Tx was provided")
	}
}
