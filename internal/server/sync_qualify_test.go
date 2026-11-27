package server

import "testing"

func TestQualifyTablePGGuard(t *testing.T) {
	pgSess := &ActiveSession{DatabaseName: "appdb"}
	cases := []struct{ db, tbl, drv, want string }{
		// physical DB name matching session active DB -> no schema prefix
		{"appdb", "users", "postgres", `"users"`},
		// default public schema -> no prefix
		{"public", "users", "postgres", `"users"`},
		// real schema name different from active DB -> keep prefix
		{"sales", "users", "postgres", `"sales"."users"`},
		// MySQL always qualifies
		{"mydb", "users", "mysql", "`mydb`.`users`"},
		// SQLite never qualifies
		{"file.db", "users", "sqlite", `"users"`},
		// empty db never qualifies
		{"", "users", "mysql", "`users`"},
	}
	for _, c := range cases {
		got := qualifyTable(pgSess, c.db, c.tbl, c.drv)
		if got != c.want {
			t.Errorf("qualifyTable(%q,%q,%q) = %q, want %q", c.db, c.tbl, c.drv, got, c.want)
		}
	}
}
