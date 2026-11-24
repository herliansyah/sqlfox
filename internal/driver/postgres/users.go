package postgres

import (
	"context"
	"fmt"
	"github.com/herliansyah/sqlfox/internal/driver"
	"regexp"
	"strings"
)

func (s *PostgresSession) GetUserList(ctx context.Context) ([]driver.DatabaseUser, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT usename FROM pg_catalog.pg_user ORDER BY usename")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []driver.DatabaseUser
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			users = append(users, driver.DatabaseUser{
				User: u,
				Host: "%",
			})
		}
	}
	return users, nil
}

func (s *PostgresSession) GetUserGrants(ctx context.Context, user, host string) ([]string, error) {
	// ponytail: Dialect-Aware Role Matrix: query role attributes and memberships from pg_roles
	var isSuper, canCreateDB, canCreateRole, canLogin, isRep, bypassRLS bool
	_ = s.db.QueryRowContext(ctx, `
		SELECT rolsuper, rolcreatedb, rolcreaterole, rolcanlogin, rolreplication, rolbypassrls
		FROM pg_catalog.pg_roles 
		WHERE rolname = $1
	`, user).Scan(&isSuper, &canCreateDB, &canCreateRole, &canLogin, &isRep, &bypassRLS)

	var grants []string
	if isSuper {
		grants = append(grants, "SUPERUSER")
	}
	if canCreateDB {
		grants = append(grants, "CREATEDB")
	}
	if canCreateRole {
		grants = append(grants, "CREATEROLE")
	}
	if canLogin {
		grants = append(grants, "LOGIN")
	}
	if isRep {
		grants = append(grants, "REPLICATION")
	}
	if bypassRLS {
		grants = append(grants, "BYPASSRLS")
	}

	// Membership in other roles
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.rolname 
		FROM pg_catalog.pg_roles r
		JOIN pg_catalog.pg_auth_members m ON m.roleid = r.oid
		JOIN pg_catalog.pg_roles u ON u.oid = m.member
		WHERE u.rolname = $1
	`, user)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var roleName string
			if err := rows.Scan(&roleName); err == nil {
				grants = append(grants, fmt.Sprintf("MEMBER OF %s", roleName))
			}
		}
	}
	if len(grants) == 0 {
		grants = []string{"CONNECT", "USAGE"}
	}
	return grants, nil
}

func (s *PostgresSession) SaveUser(ctx context.Context, user, host, password string, isNew bool) error {
	quoted := quoteIdentifier(user)
	var cmd string
	if isNew {
		cmd = fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", quoted, strings.ReplaceAll(password, "'", "''"))
	} else if password != "" {
		cmd = fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'", quoted, strings.ReplaceAll(password, "'", "''"))
	} else {
		return nil
	}
	_, err := s.db.ExecContext(ctx, cmd)
	return err
}

func (s *PostgresSession) DropUser(ctx context.Context, user, host string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP USER %s", quoteIdentifier(user)))
	return err
}

// rePrivilegeName allows only bare privilege keywords (e.g. "CONNECT", "TEMPORARY").
var rePrivilegeName = regexp.MustCompile(`^[A-Za-z]+( [A-Za-z]+)*$`)

func (s *PostgresSession) ApplyUserGrants(ctx context.Context, req driver.UserGrantRequest) error {
	quotedUser := quoteIdentifier(req.User)
	for _, priv := range req.Privileges {
		if !rePrivilegeName.MatchString(priv) {
			return fmt.Errorf("invalid privilege name: %q", priv)
		}
		cmd := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", priv, quoteIdentifier(s.cfg.DatabaseName), quotedUser)
		if _, err := s.db.ExecContext(ctx, cmd); err != nil {
			return fmt.Errorf("failed to grant %s on database %s to %s: %w", priv, s.cfg.DatabaseName, req.User, err)
		}
	}
	return nil
}
