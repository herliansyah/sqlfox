package mysql

import (
	"testing"
)

func TestParseDeadlockStatus_NoDeadlock(t *testing.T) {
	rawStatus := `
=====================================
2026-10-08 04:00:00 0x7f8a INNODB MONITOR OUTPUT
=====================================
Per second averages calculated from the last 20 seconds
-----------------
BACKGROUND THREAD
-----------------
srv_master_thread loops: 100 srv_active, 0 srv_shutdown, 2045 srv_idle
------------
TRANSACTIONS
------------
Trx id counter 1000
=====================================
END OF INNODB MONITOR OUTPUT
=====================================
`
	rep := parseDeadlockStatus(rawStatus)
	if rep.HasDeadlock {
		t.Fatalf("expected HasDeadlock false, got true")
	}
	if rep.Summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestParseDeadlockStatus_WithDeadlock(t *testing.T) {
	rawStatus := `
=====================================
2026-10-08 04:20:00 0x7f8a INNODB MONITOR OUTPUT
=====================================
------------------------
LATEST DETECTED DEADLOCK
------------------------
2026-10-08 04:19:55 0x7f8a3c007700
*** (1) TRANSACTION:
TRANSACTION 1001, ACTIVE 3 sec starting index read
mysql tables in use 1, locked 1
LOCK WAIT 2 lock struct(s), heap size 1136, 1 row lock(s)
MySQL thread id 12, OS thread handle 140232, query id 800 localhost 127.0.0.1 app_user updating
UPDATE accounts SET balance = balance - 100 WHERE id = 1
*** (1) HOLDS THE LOCK(S):
RECORD LOCKS space id 42 page no 3 n bits 72 index PRIMARY of table mydb.accounts trx id 1001 lock_mode X locks rec but not gap
*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 42 page no 4 n bits 72 index PRIMARY of table mydb.accounts trx id 1001 lock_mode X locks rec but not gap waiting
*** (2) TRANSACTION:
TRANSACTION 1002, ACTIVE 2 sec starting index read
mysql tables in use 1, locked 1
4 lock struct(s), heap size 1136, 3 row lock(s), undo log entries 2
MySQL thread id 15, OS thread handle 140235, query id 805 localhost 127.0.0.1 app_user updating
UPDATE accounts SET balance = balance + 100 WHERE id = 2
*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 42 page no 4 n bits 72 index PRIMARY of table mydb.accounts trx id 1002 lock_mode X locks rec but not gap
*** (2) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 42 page no 3 n bits 72 index PRIMARY of table mydb.accounts trx id 1002 lock_mode X locks rec but not gap waiting
*** WE ROLL BACK TRANSACTION (1)
------------
TRANSACTIONS
------------
Trx id counter 1003
`

	rep := parseDeadlockStatus(rawStatus)
	if !rep.HasDeadlock {
		t.Fatalf("expected HasDeadlock true, got false")
	}
	if len(rep.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(rep.Transactions))
	}

	tx1 := rep.Transactions[0]
	tx2 := rep.Transactions[1]

	if tx1.ThreadID != 12 {
		t.Errorf("expected tx1 ThreadID 12, got %d", tx1.ThreadID)
	}
	if tx1.TxID != "1001" {
		t.Errorf("expected tx1 TxID 1001, got %s", tx1.TxID)
	}
	if !tx1.IsVictim {
		t.Errorf("expected tx1 to be victim")
	}
	if tx2.ThreadID != 15 {
		t.Errorf("expected tx2 ThreadID 15, got %d", tx2.ThreadID)
	}
	if tx2.TxID != "1002" {
		t.Errorf("expected tx2 TxID 1002, got %s", tx2.TxID)
	}
	if tx2.IsVictim {
		t.Errorf("expected tx2 not to be victim")
	}
	if rep.VictimThreadID != 12 {
		t.Errorf("expected VictimThreadID 12, got %d", rep.VictimThreadID)
	}
}
