package db_context

// db_context_extra_test.go covers:
//   WithTransaction / Transaction — set and get
//   Transaction — not set (ok=false)
//   TxID — with and without transaction

import (
	"context"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/pkg/db/transaction"
)

// --------------------------------------------------------------------------
// WithTransaction / Transaction — happy path
// --------------------------------------------------------------------------

func TestWithTransaction_AndGet(t *testing.T) {
	tx := transaction.Build(nil, 42, false)
	ctx := WithTransaction(context.Background(), tx)
	got, ok := Transaction(ctx)
	if !ok {
		t.Fatal("Transaction() ok=false, want true")
	}
	if got != tx {
		t.Error("Transaction() returned different transaction pointer")
	}
}

// --------------------------------------------------------------------------
// Transaction — not set
// --------------------------------------------------------------------------

func TestTransaction_NotSet(t *testing.T) {
	_, ok := Transaction(context.Background())
	if ok {
		t.Error("Transaction() ok=true for empty context, want false")
	}
}

// --------------------------------------------------------------------------
// TxID — with transaction
// --------------------------------------------------------------------------

func TestTxID_WithTransaction(t *testing.T) {
	tx := transaction.Build(nil, 99, false)
	ctx := WithTransaction(context.Background(), tx)
	id, ok := TxID(ctx)
	if !ok {
		t.Fatal("TxID() ok=false, want true")
	}
	if id != 99 {
		t.Errorf("TxID() = %d, want 99", id)
	}
}

// --------------------------------------------------------------------------
// TxID — without transaction
// --------------------------------------------------------------------------

func TestTxID_WithoutTransaction(t *testing.T) {
	id, ok := TxID(context.Background())
	if ok {
		t.Error("TxID() ok=true for empty context, want false")
	}
	if id != 0 {
		t.Errorf("TxID() = %d, want 0", id)
	}
}
