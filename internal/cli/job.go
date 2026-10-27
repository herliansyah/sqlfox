package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

// JobSpec defines the declarative schema for scheduled database automation jobs.
type JobSpec struct {
	Name  string    `json:"name"`
	Tasks []JobTask `json:"tasks"`
}

// JobTask defines a single unit of work within a job pipeline.
type JobTask struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type"` // "health-check", "dump", "transfer", "query"
	Target  string `json:"target,omitempty"`
	Driver  string `json:"driver,omitempty"`
	Timeout string `json:"timeout,omitempty"`

	// Dump task configuration
	DumpType      string       `json:"dump_type,omitempty"` // "standard", "smart-seed"
	Output        string       `json:"output,omitempty"`    // destination path with {DATE}, {DATETIME}, {TIMESTAMP}
	SampleRows    int          `json:"sample_rows,omitempty"`
	IncludeSchema *bool        `json:"include_schema,omitempty"`
	IncludeData   *bool        `json:"include_data,omitempty"`
	Rotation      *JobRotation `json:"rotation,omitempty"`

	// Transfer task configuration
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	DriverFrom    string   `json:"driver_from,omitempty"`
	DriverTo      string   `json:"driver_to,omitempty"`
	Tables        []string `json:"tables,omitempty"`
	AllTables     bool     `json:"all_tables,omitempty"`
	CreateMissing bool     `json:"create_missing,omitempty"`
	Truncate      bool     `json:"truncate,omitempty"`
	BatchSize     int      `json:"batch_size,omitempty"`

	// Query task configuration
	SQL    string `json:"sql,omitempty"`
	File   string `json:"file,omitempty"`
	Format string `json:"format,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
	Force  bool   `json:"force,omitempty"`
	Tx     bool   `json:"tx,omitempty"`

	// Security & Vault overrides
	VaultDB string `json:"vault_db,omitempty"`
	Pass    string `json:"pass,omitempty"`
}

// JobRotation defines retention and pruning policies for automated dump tasks.
type JobRotation struct {
	KeepLast int    `json:"keep_last,omitempty"` // retain newest N files
	MaxAge   string `json:"max_age,omitempty"`   // e.g. "14d", "48h", "30m"
}

// PrintJobHelp writes the job subcommand help menu to w.
func PrintJobHelp(w io.Writer) {
	help := `SqlFox Job Runner - Scheduled Database Maintenance & Automation

Usage:
  sqlfox job <subcommand> [flags]

Subcommands:
  run     Execute a declarative job specification file
  check   Pre-flight validation of job syntax, targets, and output permissions
  help    Show this help message

Flags:
  --file, -f string    Path to job specification JSON file (required)
  --vault-db string    Path to config vault database (default: ~/.sqlfox/config.db)
  --pass string        Master password for vault encryption

Examples:
  sqlfox job check --file backup-job.json
  sqlfox job run --file /etc/sqlfox/nightly-sync.json
`
	fmt.Fprint(w, help)
}

// RunJob dispatches job subcommands (run, check).
func RunJob(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(args) == 0 {
		PrintJobHelp(stderr)
		return 1
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "help", "-h", "--help":
		PrintJobHelp(stdout)
		return 0
	case "run":
		return executeJobSubcommand("run", subArgs, stdout, stderr, stdin)
	case "check":
		return executeJobSubcommand("check", subArgs, stdout, stderr, stdin)
	default:
		fmt.Fprintf(stderr, "Unknown job subcommand '%s'. Run 'sqlfox job help' for usage.\n", subcmd)
		return 1
	}
}

func executeJobSubcommand(action string, args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("job "+action, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		fileFlag    = fs.String("file", "", "Path to job specification JSON file")
		shortFile   = fs.String("f", "", "Alias for --file")
		vaultDbFlag = fs.String("vault-db", "", "Path to config.db vault file")
		passFlag    = fs.String("pass", "", "Master password for vault encryption")
	)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	filePath := *fileFlag
	if filePath == "" {
		filePath = *shortFile
	}
	if filePath == "" && fs.NArg() > 0 {
		filePath = fs.Arg(0)
	}
	if filePath == "" {
		fmt.Fprintln(stderr, "❌ Error: --file <path> is required.")
		return 1
	}

	spec, err := LoadJobSpec(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "❌ Error loading job file: %v\n", err)
		return 1
	}

	ctx := context.Background()

	if action == "check" {
		return RunJobCheck(ctx, spec, *vaultDbFlag, *passFlag, stdout, stderr)
	}
	return RunJobExecution(ctx, spec, *vaultDbFlag, *passFlag, stdout, stderr, stdin)
}

// LoadJobSpec reads a JSON job file and interpolates environment variables.
func LoadJobSpec(filePath string) (*JobSpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var spec JobSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}
	expandJobSpecEnv(&spec)

	if len(spec.Tasks) == 0 {
		return nil, fmt.Errorf("job contains no tasks")
	}

	return &spec, nil
}

var reJobEnvRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expandEnv interpolates ${VAR} (empty when unset, like os.ExpandEnv) and $VAR only when VAR is set.
// Everything else is literal, so SQL dollar quotes ($$, $tag$), positional params ($1) and
// passwords containing "$" survive intact instead of being silently rewritten by os.ExpandEnv.
func expandEnv(s string) string {
	return reJobEnvRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := reJobEnvRef.FindStringSubmatch(m)
		if sub[1] != "" {
			return os.Getenv(sub[1])
		}
		if v, ok := os.LookupEnv(sub[2]); ok {
			return v
		}
		return m
	})
}

func expandJobSpecEnv(spec *JobSpec) {
	spec.Name = expandEnv(spec.Name)
	for i := range spec.Tasks {
		task := &spec.Tasks[i]
		task.Name = expandEnv(task.Name)
		task.Type = expandEnv(task.Type)
		task.Target = expandEnv(task.Target)
		task.Driver = expandEnv(task.Driver)
		task.Timeout = expandEnv(task.Timeout)
		task.DumpType = expandEnv(task.DumpType)
		task.Output = expandEnv(task.Output)
		task.From = expandEnv(task.From)
		task.To = expandEnv(task.To)
		task.DriverFrom = expandEnv(task.DriverFrom)
		task.DriverTo = expandEnv(task.DriverTo)
		for j := range task.Tables {
			task.Tables[j] = expandEnv(task.Tables[j])
		}
		task.SQL = expandEnv(task.SQL)
		task.File = expandEnv(task.File)
		task.Format = expandEnv(task.Format)
		task.VaultDB = expandEnv(task.VaultDB)
		task.Pass = expandEnv(task.Pass)
		if task.Rotation != nil {
			task.Rotation.MaxAge = expandEnv(task.Rotation.MaxAge)
		}
	}
}

// RunJobCheck performs a non-destructive dry-run validation of the job.
func RunJobCheck(ctx context.Context, spec *JobSpec, vaultPath, pass string, stdout, stderr io.Writer) int {
	fmt.Fprintf(stdout, "🔍 Checking Job: %s (%d tasks)\n", spec.Name, len(spec.Tasks))
	allPassed := true

	for i, task := range spec.Tasks {
		taskNum := i + 1
		taskDesc := task.Name
		if taskDesc == "" {
			taskDesc = fmt.Sprintf("task_%d", taskNum)
		}

		vDb := task.VaultDB
		if vDb == "" {
			vDb = vaultPath
		}
		p := task.Pass
		if p == "" {
			p = pass
		}

		switch strings.ToLower(task.Type) {
		case "health-check":
			if task.Target == "" {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] health-check requires 'target'\n", taskNum, taskDesc)
				allPassed = false
				continue
			}
			t, err := ResolveTarget(ctx, task.Target, task.Driver, vDb, p)
			if err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] health-check target unresolved: %v\n", taskNum, taskDesc, err)
				allPassed = false
			} else {
				t.Cleanup()
				fmt.Fprintf(stdout, "  ✓ [%d. %s] health-check: target reachable\n", taskNum, taskDesc)
			}

		case "dump":
			if task.Target == "" {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] dump requires 'target'\n", taskNum, taskDesc)
				allPassed = false
				continue
			}
			if task.Output == "" {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] dump requires 'output'\n", taskNum, taskDesc)
				allPassed = false
				continue
			}

			// Validate target connectivity
			t, err := ResolveTarget(ctx, task.Target, task.Driver, vDb, p)
			if err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] dump target unresolved: %v\n", taskNum, taskDesc, err)
				allPassed = false
				continue
			}
			t.Cleanup()

			// Validate output directory writability
			resolvedOut := InterpolatePathTokens(task.Output)
			outDir := filepath.Dir(resolvedOut)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] output directory cannot be created: %v\n", taskNum, taskDesc, err)
				allPassed = false
				continue
			}
			testFile := filepath.Join(outDir, fmt.Sprintf(".sqlfox_test_%d", time.Now().UnixNano()))
			if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] output directory not writable: %v\n", taskNum, taskDesc, err)
				allPassed = false
			} else {
				_ = os.Remove(testFile)
				fmt.Fprintf(stdout, "  ✓ [%d. %s] dump: target OK, output path writable (%s)\n", taskNum, taskDesc, task.Output)
			}

		case "query":
			if task.Target == "" {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] query requires 'target'\n", taskNum, taskDesc)
				allPassed = false
				continue
			}
			if task.SQL == "" && task.File == "" {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] query requires either 'sql' or 'file'\n", taskNum, taskDesc)
				allPassed = false
				continue
			}
			if task.File != "" {
				if _, err := os.Stat(task.File); err != nil {
					fmt.Fprintf(stderr, "  ❌ [%d. %s] script file '%s' not found: %v\n", taskNum, taskDesc, task.File, err)
					allPassed = false
					continue
				}
			}
			t, err := ResolveTarget(ctx, task.Target, task.Driver, vDb, p)
			if err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] query target unresolved: %v\n", taskNum, taskDesc, err)
				allPassed = false
			} else {
				t.Cleanup()
				fmt.Fprintf(stdout, "  ✓ [%d. %s] query: target OK, query inputs valid\n", taskNum, taskDesc)
			}

		case "transfer":
			if task.From == "" || task.To == "" {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] transfer requires both 'from' and 'to'\n", taskNum, taskDesc)
				allPassed = false
				continue
			}
			tFrom, err := ResolveTarget(ctx, task.From, task.DriverFrom, vDb, p)
			if err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] transfer source error: %v\n", taskNum, taskDesc, err)
				allPassed = false
				continue
			}
			tFrom.Cleanup()

			tTo, err := ResolveTarget(ctx, task.To, task.DriverTo, vDb, p)
			if err != nil {
				fmt.Fprintf(stderr, "  ❌ [%d. %s] transfer destination error: %v\n", taskNum, taskDesc, err)
				allPassed = false
				continue
			}
			tTo.Cleanup()
			fmt.Fprintf(stdout, "  ✓ [%d. %s] transfer: source and destination reachable\n", taskNum, taskDesc)

		default:
			fmt.Fprintf(stderr, "  ❌ [%d. %s] unknown task type: '%s'\n", taskNum, taskDesc, task.Type)
			allPassed = false
		}
	}

	if allPassed {
		fmt.Fprintln(stdout, "✅ All pre-flight checks passed successfully.")
		return 0
	}
	fmt.Fprintln(stderr, "❌ Pre-flight checks failed.")
	return 1
}

// RunJobExecution executes tasks sequentially with fail-fast semantics.
func RunJobExecution(ctx context.Context, spec *JobSpec, vaultPath, pass string, stdout, stderr io.Writer, stdin io.Reader) int {
	startTime := time.Now()
	fmt.Fprintf(stderr, "🚀 Starting Job: %s (%d tasks) at %s\n", spec.Name, len(spec.Tasks), startTime.Format(time.RFC3339))

	for i, task := range spec.Tasks {
		taskNum := i + 1
		taskDesc := task.Name
		if taskDesc == "" {
			taskDesc = fmt.Sprintf("task_%d", taskNum)
		}

		vDb := task.VaultDB
		if vDb == "" {
			vDb = vaultPath
		}
		p := task.Pass
		if p == "" {
			p = pass
		}

		t0 := time.Now()
		fmt.Fprintf(stderr, "▶ [%d/%d] Executing: %s (%s)...\n", taskNum, len(spec.Tasks), taskDesc, task.Type)

		var exitCode int
		switch strings.ToLower(task.Type) {
		case "health-check":
			exitCode = executeHealthCheckTask(ctx, task, vDb, p, stderr)
		case "dump":
			exitCode = executeDumpTask(ctx, task, vDb, p, stdout, stderr)
		case "query":
			exitCode = executeQueryTask(ctx, task, vDb, p, stdout, stderr, stdin)
		case "transfer":
			exitCode = executeTransferTask(ctx, task, vDb, p, stdout, stderr, stdin)
		default:
			fmt.Fprintf(stderr, "❌ Unknown task type '%s'\n", task.Type)
			exitCode = 1
		}

		elapsed := time.Since(t0)
		if exitCode != 0 {
			fmt.Fprintf(stderr, "❌ Task [%d/%d] failed with exit code %d after %v. Aborting job.\n", taskNum, len(spec.Tasks), exitCode, elapsed)
			return exitCode
		}
		fmt.Fprintf(stderr, "✓ [%d/%d] Completed in %v\n", taskNum, len(spec.Tasks), elapsed)
	}

	totalDuration := time.Since(startTime)
	fmt.Fprintf(stderr, "✨ Job '%s' completed successfully in %v.\n", spec.Name, totalDuration)
	return 0
}

func executeHealthCheckTask(ctx context.Context, task JobTask, vaultPath, pass string, stderr io.Writer) int {
	timeout := 10 * time.Second
	if task.Timeout != "" {
		if d, err := time.ParseDuration(task.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	cCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	t, err := ResolveTarget(cCtx, task.Target, task.Driver, vaultPath, pass)
	if err != nil {
		fmt.Fprintf(stderr, "  ❌ Connection failed: %v\n", err)
		return 1
	}
	defer t.Cleanup()

	// Execute quick ping query
	t0 := time.Now()
	res, err := t.Session.Exec(cCtx, "SELECT 1")
	if err != nil {
		// Fallback to QueryStream in case driver Exec dislikes SELECT
		stream, qErr := t.Session.QueryStream(cCtx, "SELECT 1", 1)
		if qErr != nil {
			fmt.Fprintf(stderr, "  ❌ Ping failed: %v\n", err)
			return 1
		}
		_ = stream.Close()
	}
	_ = res

	pingDuration := time.Since(t0)
	fmt.Fprintf(stderr, "  ✓ Target '%s' is healthy (ping latency: %v)\n", task.Target, pingDuration)
	return 0
}

func executeDumpTask(ctx context.Context, task JobTask, vaultPath, pass string, stdout, stderr io.Writer) int {
	if task.Target == "" || task.Output == "" {
		fmt.Fprintln(stderr, "  ❌ dump task requires both 'target' and 'output'")
		return 1
	}

	timeout := 30 * time.Minute
	if task.Timeout != "" {
		if d, err := time.ParseDuration(task.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	cCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	t, err := ResolveTarget(cCtx, task.Target, task.Driver, vaultPath, pass)
	if err != nil {
		fmt.Fprintf(stderr, "  ❌ Failed to connect to target: %v\n", err)
		return 1
	}
	defer t.Cleanup()

	resolvedOut := InterpolatePathTokens(task.Output)
	outDir := filepath.Dir(resolvedOut)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(stderr, "  ❌ Failed to create output directory: %v\n", err)
		return 1
	}

	// Write to a temp file in the same directory and atomically rename only after a complete,
	// flushed dump: a failed run never truncates an existing dump, and rotation only runs on success.
	file, err := os.CreateTemp(outDir, "."+filepath.Base(resolvedOut)+".tmp-*")
	if err != nil {
		fmt.Fprintf(stderr, "  ❌ Failed to create output file '%s': %v\n", resolvedOut, err)
		return 1
	}
	tmpPath := file.Name()
	committed := false
	defer func() {
		if !committed {
			_ = file.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	includeSchema := true
	if task.IncludeSchema != nil {
		includeSchema = *task.IncludeSchema
	}
	includeData := true
	if task.IncludeData != nil {
		includeData = *task.IncludeData
	}

	sampleLimit := task.SampleRows
	if sampleLimit <= 0 {
		sampleLimit = 20
	}

	fmt.Fprintf(stderr, "  💾 Exporting dump to: %s\n", resolvedOut)
	bw := bufio.NewWriterSize(file, 256*1024)
	var dumpErr error
	if strings.ToLower(task.DumpType) == "smart-seed" {
		dumpErr = WriteSmartSeedDump(cCtx, t.Session, t.Driver, t.DBName, sampleLimit, includeSchema, bw)
	} else {
		dumpErr = WriteStandardDump(cCtx, t.Session, t.Driver, t.DBName, includeSchema, includeData, bw)
	}
	if dumpErr == nil {
		dumpErr = bw.Flush()
	}
	if dumpErr == nil {
		dumpErr = file.Sync()
	}
	if dumpErr == nil {
		dumpErr = file.Close()
	}
	if dumpErr == nil {
		_ = os.Chmod(tmpPath, 0644)
		dumpErr = os.Rename(tmpPath, resolvedOut)
	}
	if dumpErr != nil {
		fmt.Fprintf(stderr, "  ❌ Dump generation failed: %v\n", dumpErr)
		return 1
	}
	committed = true

	// Apply retention rotation if configured
	if task.Rotation != nil {
		ApplyDumpRotation(outDir, task.Output, task.Rotation, stderr)
	}

	return 0
}

func executeQueryTask(ctx context.Context, task JobTask, vaultPath, pass string, stdout, stderr io.Writer, stdin io.Reader) int {
	var args []string

	if strings.Contains(task.Target, "://") || strings.HasPrefix(task.Target, "./") || strings.HasPrefix(task.Target, "/") || strings.Contains(task.Target, "@tcp(") {
		args = append(args, "--dsn", task.Target)
	} else {
		args = append(args, "--conn", task.Target)
	}

	if task.Driver != "" {
		args = append(args, "--driver", task.Driver)
	}
	if task.File != "" {
		args = append(args, "--file", task.File)
	}
	if task.Format != "" {
		args = append(args, "--format", task.Format)
	}
	if task.Limit != nil {
		args = append(args, "--limit", strconv.Itoa(*task.Limit))
	}
	if task.Force {
		args = append(args, "--force")
	}
	if task.Tx {
		args = append(args, "--tx")
	}
	if vaultPath != "" {
		args = append(args, "--vault-db", vaultPath)
	}
	if pass != "" {
		args = append(args, "--pass", pass)
	}

	if task.SQL != "" {
		// "--" ends flag parsing so SQL starting with "-" (e.g. a "-- comment" header) stays positional.
		args = append(args, "--", task.SQL)
	}

	outWriter := stdout
	var closeOut func()
	if task.Output != "" {
		resolvedOut := InterpolatePathTokens(task.Output)
		if err := os.MkdirAll(filepath.Dir(resolvedOut), 0755); err != nil {
			fmt.Fprintf(stderr, "  ❌ Cannot create query output directory: %v\n", err)
			return 1
		}
		f, err := os.OpenFile(resolvedOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			fmt.Fprintf(stderr, "  ❌ Cannot open query output file: %v\n", err)
			return 1
		}
		outWriter = f
		closeOut = func() { _ = f.Close() }
	}
	if closeOut != nil {
		defer closeOut()
	}

	return RunQuery(args, outWriter, stderr, stdin)
}

func executeTransferTask(ctx context.Context, task JobTask, vaultPath, pass string, stdout, stderr io.Writer, stdin io.Reader) int {
	var args []string
	args = append(args, "--from", task.From, "--to", task.To)

	if task.DriverFrom != "" {
		args = append(args, "--driver-from", task.DriverFrom)
	}
	if task.DriverTo != "" {
		args = append(args, "--driver-to", task.DriverTo)
	}
	if task.AllTables {
		args = append(args, "--all-tables")
	} else if len(task.Tables) > 0 {
		args = append(args, "--tables", strings.Join(task.Tables, ","))
	}
	if task.CreateMissing {
		args = append(args, "--create-missing")
	}
	if task.Truncate {
		args = append(args, "--truncate")
	}
	if task.BatchSize > 0 {
		args = append(args, "--batch-size", strconv.Itoa(task.BatchSize))
	}
	if vaultPath != "" {
		args = append(args, "--vault-db", vaultPath)
	}
	if pass != "" {
		args = append(args, "--pass", pass)
	}

	return RunTransfer(args, stdout, stderr, stdin)
}

// InterpolatePathTokens replaces {DATE}, {DATETIME}, and {TIMESTAMP} with formatted values.
func InterpolatePathTokens(pattern string) string {
	now := time.Now()
	res := strings.ReplaceAll(pattern, "{DATE}", now.Format("2006-01-02"))
	res = strings.ReplaceAll(res, "{DATETIME}", now.Format("20060102-150405"))
	res = strings.ReplaceAll(res, "{TIMESTAMP}", strconv.FormatInt(now.Unix(), 10))
	return res
}

// rotationPattern builds an exact filename matcher for a dump template: literal parts must match
// verbatim and each {DATE}/{DATETIME}/{TIMESTAMP} token only matches its own format, so rotation can
// never touch another job's dumps or unrelated files. Returns nil for token-less templates (a single
// fixed file that is simply overwritten, nothing to rotate).
func rotationPattern(baseTemplate string) *regexp.Regexp {
	tokenRe := regexp.MustCompile(`\{(DATE|DATETIME|TIMESTAMP)\}`)
	if !tokenRe.MatchString(baseTemplate) {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("^")
	last := 0
	for _, loc := range tokenRe.FindAllStringSubmatchIndex(baseTemplate, -1) {
		sb.WriteString(regexp.QuoteMeta(baseTemplate[last:loc[0]]))
		switch baseTemplate[loc[2]:loc[3]] {
		case "DATE":
			sb.WriteString(`\d{4}-\d{2}-\d{2}`)
		case "DATETIME":
			sb.WriteString(`\d{8}-\d{6}`)
		case "TIMESTAMP":
			sb.WriteString(`\d+`)
		}
		last = loc[1]
	}
	sb.WriteString(regexp.QuoteMeta(baseTemplate[last:]))
	sb.WriteString("$")
	return regexp.MustCompile(sb.String())
}

// ApplyDumpRotation prunes older matching dump files according to keep_last and max_age.
// Only files whose names match this job's template exactly (see rotationPattern) are considered.
func ApplyDumpRotation(dir, template string, rot *JobRotation, stderr io.Writer) {
	if rot == nil || (rot.KeepLast <= 0 && rot.MaxAge == "") {
		return
	}

	pattern := rotationPattern(filepath.Base(template))
	if pattern == nil {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type fileMeta struct {
		name    string
		path    string
		modTime time.Time
	}

	var matching []fileMeta
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if pattern.MatchString(name) {
			info, err := entry.Info()
			if err == nil {
				matching = append(matching, fileMeta{
					name:    name,
					path:    filepath.Join(dir, name),
					modTime: info.ModTime(),
				})
			}
		}
	}

	if len(matching) == 0 {
		return
	}

	// Sort newest first
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].modTime.After(matching[j].modTime)
	})

	var maxDuration time.Duration
	if rot.MaxAge != "" {
		ageStr := strings.TrimSpace(rot.MaxAge)
		if strings.HasSuffix(ageStr, "d") {
			if days, err := strconv.Atoi(strings.TrimSuffix(ageStr, "d")); err == nil && days > 0 {
				maxDuration = time.Duration(days) * 24 * time.Hour
			}
		} else if d, err := time.ParseDuration(ageStr); err == nil && d > 0 {
			maxDuration = d
		}
	}

	for i, f := range matching {
		shouldDelete := (rot.KeepLast > 0 && i >= rot.KeepLast) || (maxDuration > 0 && time.Since(f.modTime) > maxDuration)
		if shouldDelete {
			if err := os.Remove(f.path); err == nil {
				fmt.Fprintf(stderr, "  ♻️  Rotated expired dump: %s\n", f.name)
			}
		}
	}
}

// WriteStandardDump writes DDL and data inserts for all tables to w.
func WriteStandardDump(ctx context.Context, sess driver.Session, drvName, dbName string, includeSchema, includeData bool, w io.Writer) error {
	return writeTableDump(ctx, sess, drvName, dbName, false, 0, includeSchema, includeData, w)
}

// WriteSmartSeedDump writes topologically sorted DDL and sampled records for testing/staging.
func WriteSmartSeedDump(ctx context.Context, sess driver.Session, drvName, dbName string, limit int, includeSchema bool, w io.Writer) error {
	return writeTableDump(ctx, sess, drvName, dbName, true, limit, includeSchema, true, w)
}

// errWriter remembers the first write error so dump code can write freely and check once.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err == nil {
		_, e.err = fmt.Fprintf(e.w, format, args...)
	}
}

func writeTableDump(ctx context.Context, sess driver.Session, drvName, dbName string, isSmartSeed bool, limit int, includeSchema, includeData bool, w io.Writer) error {
	schema, err := sess.InspectSchema(ctx, dbName)
	if err != nil {
		return fmt.Errorf("inspect schema: %w", err)
	}

	tables := schema.Tables
	title := "SqlFox Standard SQL Dump"
	if isSmartSeed {
		title = fmt.Sprintf("SqlFox FK-Aware Smart Seed Dump (Limit: %d rows)", limit)
	}
	// FK parents first and views last, so the dump restores on engines without a FK-check toggle (PostgreSQL).
	tables = driver.TopologicalSortTables(tables)
	var baseTables, views []driver.TableMetadata
	isView := func(t driver.TableMetadata) bool { return strings.HasSuffix(strings.ToUpper(t.Type), "VIEW") }
	for _, t := range tables {
		if isView(t) {
			views = append(views, t)
		} else {
			baseTables = append(baseTables, t)
		}
	}
	tables = append(baseTables, views...)

	quoteIdent := func(s string) string { return driver.QuoteIdentifier(s, drvName) }
	isPg := drvName == "postgres" || drvName == "postgresql" || drvName == "pg"
	out := &errWriter{w: w}

	out.printf("-- %s\n-- Database: %s\n-- Dump Date: %s\n\n", title, dbName, time.Now().Format(time.RFC3339))
	if drvName == "mysql" {
		out.printf("SET FOREIGN_KEY_CHECKS = 0;\nSET SQL_MODE = 'NO_AUTO_VALUE_ON_ZERO';\n\n")
	} else if drvName == "sqlite" {
		out.printf("PRAGMA foreign_keys = OFF;\n\n")
	}

	chunkBatch := 100
	if isSmartSeed && limit > 0 && limit < chunkBatch {
		chunkBatch = limit
	}

	if includeSchema {
		// Drop in reverse dependency order so children go before their parents.
		for i := len(tables) - 1; i >= 0; i-- {
			kind := "TABLE"
			if isView(tables[i]) {
				kind = "VIEW"
				if strings.Contains(strings.ToUpper(tables[i].Type), "MATERIALIZED") {
					kind = "MATERIALIZED VIEW"
				}
			}
			cascade := ""
			if isPg {
				cascade = " CASCADE"
			}
			out.printf("DROP %s IF EXISTS %s%s;\n", kind, quoteIdent(tables[i].Name), cascade)
		}
		out.printf("\n")
	}

	for _, tbl := range tables {
		if err := ctx.Err(); err != nil {
			return err
		}
		if includeSchema {
			ddl, err := sess.GetTableDDL(ctx, tbl.Name)
			if err != nil {
				return fmt.Errorf("get DDL for %s: %w", tbl.Name, err)
			}
			if ddl != "" {
				out.printf("%s;\n\n", strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
			}
		}

		if isView(tbl) || !includeData {
			continue
		}

		querySQL := fmt.Sprintf("SELECT * FROM %s", quoteIdent(tbl.Name))
		if isSmartSeed && limit > 0 {
			querySQL = fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoteIdent(tbl.Name), limit)
		}

		streamLimit := 0
		if isSmartSeed {
			streamLimit = limit
		}
		stream, err := sess.QueryStream(ctx, querySQL, streamLimit)
		if err != nil {
			return fmt.Errorf("read rows of %s: %w", tbl.Name, err)
		}

		cols := stream.Columns()
		var colNames []string
		for _, c := range cols {
			colNames = append(colNames, quoteIdent(c.Name))
		}
		colHeader := strings.Join(colNames, ", ")

		written := 0
		for {
			chunk, err := stream.NextChunk(chunkBatch)
			if len(chunk) > 0 {
				var valRows []string
				for _, row := range chunk {
					var rowVals []string
					for _, val := range row {
						rowVals = append(rowVals, driver.FormatSQLValueForDriver(val, drvName))
					}
					valRows = append(valRows, fmt.Sprintf("  (%s)", strings.Join(rowVals, ", ")))
				}
				out.printf("INSERT INTO %s (%s) VALUES\n%s;\n\n", quoteIdent(tbl.Name), colHeader, strings.Join(valRows, ",\n"))
				written += len(chunk)
			}
			if err == io.EOF || (err == nil && len(chunk) == 0) {
				break
			}
			if err != nil {
				_ = stream.Close()
				return fmt.Errorf("read rows of %s: %w", tbl.Name, err)
			}
			if out.err != nil || (isSmartSeed && limit > 0 && written >= limit) {
				break
			}
		}
		_ = stream.Close()
		for _, stmt := range driver.SequenceResetStatements(tbl, drvName) {
			out.printf("%s\n\n", stmt)
		}
		if out.err != nil {
			return fmt.Errorf("write dump: %w", out.err)
		}
	}

	if drvName == "mysql" {
		out.printf("SET FOREIGN_KEY_CHECKS = 1;\n")
	} else if drvName == "sqlite" {
		out.printf("PRAGMA foreign_keys = ON;\n")
	}
	if out.err != nil {
		return fmt.Errorf("write dump: %w", out.err)
	}
	return nil
}
