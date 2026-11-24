package mysql

import (
	"context"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"strings"
)

func (s *MySQLSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT user, host FROM mysql.user ORDER BY user ASC, host ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to list database users: %w", err)
	}
	defer rows.Close()

	var users []driver.DatabaseUser
	for rows.Next() {
		var u driver.DatabaseUser
		if err := rows.Scan(&u.User, &u.Host); err == nil {
			users = append(users, u)
		}
	}
	if users == nil {
		users = []driver.DatabaseUser{}
	}
	return users, nil
}

func (s *MySQLSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	sqlCmd := fmt.Sprintf("SHOW GRANTS FOR %s@%s", driver.EscapeSQLString(user, "mysql"), driver.EscapeSQLString(host, "mysql"))
	rows, err := s.db.QueryContext(ctx, sqlCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user grants: %w", err)
	}
	defer rows.Close()

	var grants []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err == nil {
			grants = append(grants, g)
		}
	}
	if grants == nil {
		grants = []string{}
	}
	return grants, nil
}

func (s *MySQLSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	uEsc := driver.EscapeSQLString(user, "mysql")
	hEsc := driver.EscapeSQLString(host, "mysql")
	pEsc := driver.EscapeSQLString(password, "mysql")

	if isNew {
		cmd := fmt.Sprintf("CREATE USER %s@%s IDENTIFIED BY %s", uEsc, hEsc, pEsc)
		if _, err := s.db.ExecContext(ctx, cmd); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	} else if password != "" {
		cmd := fmt.Sprintf("ALTER USER %s@%s IDENTIFIED BY %s", uEsc, hEsc, pEsc)
		if _, err := s.db.ExecContext(ctx, cmd); err != nil {
			return fmt.Errorf("failed to update user password: %w", err)
		}
	}
	_, _ = s.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}

func (s *MySQLSession) DropUser(ctx context.Context, user, host string) error {
	uEsc := driver.EscapeSQLString(user, "mysql")
	hEsc := driver.EscapeSQLString(host, "mysql")
	cmd := fmt.Sprintf("DROP USER %s@%s", uEsc, hEsc)
	if _, err := s.db.ExecContext(ctx, cmd); err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}

func (s *MySQLSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	uEsc := driver.EscapeSQLString(req.User, "mysql")
	hEsc := driver.EscapeSQLString(req.Host, "mysql")
	for _, p := range req.Privileges {
		if !driver.ValidPrivilegeName(p) {
			return fmt.Errorf("invalid privilege name: %q", p)
		}
	}
	privs := strings.Join(req.Privileges, ", ")
	if privs == "" {
		privs = "USAGE"
	}

	target := "*.*"
	if req.Scope == "DATABASE" && req.Database != "" {
		target = driver.QuoteIdentifier(req.Database, "mysql") + ".*"
	}

	cmd := fmt.Sprintf("GRANT %s ON %s TO %s@%s", privs, target, uEsc, hEsc)
	if req.WithGrant {
		cmd += " WITH GRANT OPTION"
	}

	if _, err := s.db.ExecContext(ctx, cmd); err != nil {
		return fmt.Errorf("failed to apply user grants: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}
