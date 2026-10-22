// Package store keeps every notebook record in one SQLite file.
//
// Documents are stored as opaque JSON objects; the server does not inspect
// their content (SPEC §5, SPEC2 §4). The only fields the store touches are "id" (set to
// the document key) and nothing else.
package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure Go SQLite driver (no CGO)
)

// Limits from SPEC §4.
const (
	MaxDocBytes   = 256 << 10
	MaxDocsPerCol = 20000
	SchemaVersion = "1"
	settingsColl  = "settings"
	settingsID    = "main"
)

// Collections that the HTTP API may touch directly, in a fixed order
// (state, export and import responses list them in this order).
var Collections = []string{
	"notes", "blockers", "fridays", // 1.0
	"requests", "programs", "decisions", "mapitems", "timelog", "weeks", "cycles", "risks", // 2.0
}

// V1Collections are the collections a 1.0 export file holds.
var V1Collections = []string{"notes", "blockers", "fridays"}

var (
	ErrNotFound   = errors.New("not found")
	ErrBadID      = errors.New("bad id")
	ErrBadColl    = errors.New("bad collection")
	ErrTooLarge   = errors.New("document too large")
	ErrLimit      = errors.New("collection limit reached")
	ErrNotObject  = errors.New("document must be a JSON object")
	ErrBadFormat  = errors.New("bad export format")
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)
	validCollName = func() map[string]bool {
		m := map[string]bool{}
		for _, c := range Collections {
			m[c] = true
		}
		return m
	}()
)

// Doc is one JSON object. It always carries "id" when returned by the store.
type Doc = map[string]any

// ValidColl reports whether name is one of the API collections.
func ValidColl(name string) bool { return validCollName[name] }

// ValidID reports whether id matches [A-Za-z0-9_.:-]{1,64}.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Store wraps the SQLite database.
type Store struct {
	db  *sql.DB
	now func() time.Time

	idMu   sync.Mutex
	lastMS int64
	seq    uint32
}

// Open opens (or creates) dataDir/notebook.db and runs migrations.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dataDir, "notebook.db")
	return OpenDSN("file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
}

// OpenDSN opens a database by DSN (tests use this with a temp dir).
func OpenDSN(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One writer is enough for a single-user notebook and avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Ping checks the database connection.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS docs (
			coll TEXT NOT NULL,
			id TEXT NOT NULL,
			data TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (coll, id)
		)`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO meta(key, value) VALUES('schema_version', '` + SchemaVersion + `')
			ON CONFLICT(key) DO NOTHING`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// SchemaVersion returns the stored schema version.
func (s *Store) Schema(ctx context.Context) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&v)
	return v, err
}

// NewID returns a time-ordered id: 12 hex digits of Unix milliseconds,
// a 4-hex sequence for ids made in the same millisecond, and 6 random hex
// digits. Lexical order equals creation order on one server.
func (s *Store) NewID() string {
	s.idMu.Lock()
	ms := s.now().UnixMilli()
	if ms <= s.lastMS {
		ms = s.lastMS
		s.seq++
	} else {
		s.lastMS = ms
		s.seq = 0
	}
	seq := s.seq
	s.idMu.Unlock()
	var r [3]byte
	_, _ = rand.Read(r[:])
	return fmt.Sprintf("%012x-%04x-%s", ms, seq&0xffff, hex.EncodeToString(r[:]))
}

func (s *Store) stamp() string { return s.now().UTC().Format(time.RFC3339Nano) }

func encode(id string, d Doc) (string, error) {
	if d == nil {
		return "", ErrNotObject
	}
	d["id"] = id
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	if len(b) > MaxDocBytes {
		return "", ErrTooLarge
	}
	return string(b), nil
}

func decode(data string) (Doc, error) {
	var d Doc
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return nil, err
	}
	if d == nil {
		d = Doc{}
	}
	return d, nil
}

type querier interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func countColl(ctx context.Context, q querier, coll string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs WHERE coll=?`, coll).Scan(&n)
	return n, err
}

func exists(ctx context.Context, q querier, coll, id string) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM docs WHERE coll=? AND id=?`, coll, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Create stores d under a new id and returns the stored doc.
func (s *Store) Create(ctx context.Context, coll string, d Doc) (Doc, error) {
	if !ValidColl(coll) {
		return nil, ErrBadColl
	}
	return s.Put(ctx, coll, s.NewID(), d)
}

// Put replaces (or creates) coll/id with d.
func (s *Store) Put(ctx context.Context, coll, id string, d Doc) (Doc, error) {
	if !ValidColl(coll) {
		return nil, ErrBadColl
	}
	return s.put(ctx, coll, id, d)
}

func (s *Store) put(ctx context.Context, coll, id string, d Doc) (Doc, error) {
	if !ValidID(id) {
		return nil, ErrBadID
	}
	data, err := encode(id, d)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := exists(ctx, tx, coll, id)
	if err != nil {
		return nil, err
	}
	if !ok && coll != settingsColl {
		n, err := countColl(ctx, tx, coll)
		if err != nil {
			return nil, err
		}
		if n >= MaxDocsPerCol {
			return nil, ErrLimit
		}
	}
	now := s.stamp()
	_, err = tx.ExecContext(ctx, `INSERT INTO docs(coll, id, data, created_at, updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(coll, id) DO UPDATE SET data=excluded.data, updated_at=excluded.updated_at`,
		coll, id, data, now, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d, nil
}

// Patch merges top-level keys of p into coll/id. A null value deletes the key.
// The "id" key cannot be changed or removed.
func (s *Store) Patch(ctx context.Context, coll, id string, p Doc) (Doc, error) {
	if !ValidColl(coll) {
		return nil, ErrBadColl
	}
	if !ValidID(id) {
		return nil, ErrBadID
	}
	if p == nil {
		return nil, ErrNotObject
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var data string
	err = tx.QueryRowContext(ctx, `SELECT data FROM docs WHERE coll=? AND id=?`, coll, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cur, err := decode(data)
	if err != nil {
		return nil, err
	}
	for k, v := range p {
		if k == "id" {
			continue
		}
		if v == nil {
			delete(cur, k)
		} else {
			cur[k] = v
		}
	}
	out, err := encode(id, cur)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE docs SET data=?, updated_at=? WHERE coll=? AND id=?`,
		out, s.stamp(), coll, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return cur, nil
}

// Get returns coll/id or ErrNotFound.
func (s *Store) Get(ctx context.Context, coll, id string) (Doc, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM docs WHERE coll=? AND id=?`, coll, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decode(data)
}

// Delete removes coll/id. Deleting a missing doc is not an error.
func (s *Store) Delete(ctx context.Context, coll, id string) error {
	if !ValidColl(coll) {
		return ErrBadColl
	}
	if !ValidID(id) {
		return ErrBadID
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM docs WHERE coll=? AND id=?`, coll, id)
	return err
}

// List returns all docs of coll ordered by id.
func (s *Store) List(ctx context.Context, coll string) ([]Doc, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM docs WHERE coll=? ORDER BY id`, coll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Doc{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		d, err := decode(data)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Settings returns the settings object or nil when none was saved.
func (s *Store) Settings(ctx context.Context) (Doc, error) {
	d, err := s.Get(ctx, settingsColl, settingsID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	delete(d, "id")
	return d, nil
}

// PutSettings replaces the settings object.
func (s *Store) PutSettings(ctx context.Context, d Doc) (Doc, error) {
	if d == nil {
		return nil, ErrNotObject
	}
	cp := make(Doc, len(d))
	for k, v := range d {
		cp[k] = v
	}
	if _, err := s.put(ctx, settingsColl, settingsID, cp); err != nil {
		return nil, err
	}
	delete(cp, "id")
	return cp, nil
}

// State is everything the front end needs on load: the settings (nil when
// never saved) and every collection (never nil, maybe empty).
type State struct {
	Settings Doc
	Colls    map[string][]Doc
}

// LoadState reads settings and every collection.
func (s *Store) LoadState(ctx context.Context) (*State, error) {
	st := &State{Colls: make(map[string][]Doc, len(Collections))}
	var err error
	if st.Settings, err = s.Settings(ctx); err != nil {
		return nil, err
	}
	for _, c := range Collections {
		if st.Colls[c], err = s.List(ctx, c); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// ExportFormat is the "format" value of an export file.
const ExportFormat = "manager-notebook-export"

// Export is the export file (SPEC2 §3): format, version, exportedAt,
// settings and one flat key per collection.
type Export struct {
	Format     string
	Version    string
	ExportedAt string
	Settings   Doc
	Colls      map[string][]Doc
}

// MarshalJSON writes the keys in a fixed order with every collection present.
func (e *Export) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	put := func(k string, v any) error {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
		return nil
	}
	if err := put("format", e.Format); err != nil {
		return nil, err
	}
	if err := put("version", e.Version); err != nil {
		return nil, err
	}
	if err := put("exportedAt", e.ExportedAt); err != nil {
		return nil, err
	}
	if err := put("settings", e.Settings); err != nil {
		return nil, err
	}
	for _, c := range Collections {
		docs := e.Colls[c]
		if docs == nil {
			docs = []Doc{}
		}
		if err := put(c, docs); err != nil {
			return nil, err
		}
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// MakeExport builds an export of the current data.
func (s *Store) MakeExport(ctx context.Context, version string) (*Export, error) {
	st, err := s.LoadState(ctx)
	if err != nil {
		return nil, err
	}
	return &Export{
		Format:     ExportFormat,
		Version:    version,
		ExportedAt: s.now().Format(time.RFC3339),
		Settings:   st.Settings,
		Colls:      st.Colls,
	}, nil
}

// ImportFile is a parsed import file. Only the collections whose key was in
// the file are in Colls; Settings is nil when the key was missing or null.
type ImportFile struct {
	Settings Doc
	Colls    map[string][]Doc
}

// ParseImport reads an import file that was first split into its top-level
// keys, so a missing key can be told apart from an empty one (SPEC2 §3):
//   - "format" must be ExportFormat, otherwise ErrBadFormat.
//   - a collection key present with an array replaces that collection
//     (an empty array empties it); missing or null keeps it.
//   - "settings" present and not null replaces the settings.
//
// A 1.0 export holds only notes, blockers, fridays and settings, so the 2.0
// collections are kept. Unknown keys are ignored.
func ParseImport(raw map[string]json.RawMessage) (*ImportFile, error) {
	if raw == nil {
		return nil, ErrBadFormat
	}
	var format string
	if err := json.Unmarshal(raw["format"], &format); err != nil || format != ExportFormat {
		return nil, ErrBadFormat
	}
	f := &ImportFile{Colls: map[string][]Doc{}}
	if v, ok := raw["settings"]; ok && !isNull(v) {
		var d Doc
		if err := decodeNumber(v, &d); err != nil || d == nil {
			return nil, ErrBadFormat
		}
		f.Settings = d
	}
	for _, c := range Collections {
		v, ok := raw[c]
		if !ok || isNull(v) {
			continue
		}
		var docs []Doc
		if err := decodeNumber(v, &docs); err != nil {
			return nil, ErrBadFormat
		}
		if docs == nil {
			docs = []Doc{}
		}
		f.Colls[c] = docs
	}
	return f, nil
}

func isNull(v json.RawMessage) bool {
	return len(bytes.TrimSpace(v)) == 0 || string(bytes.TrimSpace(v)) == "null"
}

// decodeNumber decodes with UseNumber so large numbers survive unchanged.
func decodeNumber(v json.RawMessage, out any) error {
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	return dec.Decode(out)
}

// ImportResult is the /api/import response: the collections that were
// replaced (in Collections order), their new counts, and whether the
// settings were replaced.
type ImportResult struct {
	Replaced []string       `json:"replaced"`
	Counts   map[string]int `json:"counts"`
	Settings bool           `json:"settings"`
}

// Import replaces the collections present in f (and the settings when given)
// in one transaction. Docs without a valid or unique "id" get a new one.
// On any error nothing is changed.
func (s *Store) Import(ctx context.Context, f *ImportFile) (*ImportResult, error) {
	if f == nil {
		return nil, ErrBadFormat
	}
	type row struct{ coll, id, data string }
	var rows []row
	res := &ImportResult{Replaced: []string{}, Counts: map[string]int{}}
	for _, coll := range Collections {
		docs, ok := f.Colls[coll]
		if !ok {
			continue
		}
		if len(docs) > MaxDocsPerCol {
			return nil, ErrLimit
		}
		seen := map[string]bool{}
		for _, d := range docs {
			if d == nil {
				return nil, ErrNotObject
			}
			id, _ := d["id"].(string)
			if !ValidID(id) || seen[id] {
				id = s.NewID()
			}
			seen[id] = true
			data, err := encode(id, d)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row{coll, id, data})
		}
		res.Replaced = append(res.Replaced, coll)
		res.Counts[coll] = len(docs)
	}
	var settingsData string
	if f.Settings != nil {
		cp := make(Doc, len(f.Settings))
		for k, v := range f.Settings {
			cp[k] = v
		}
		data, err := encode(settingsID, cp)
		if err != nil {
			return nil, err
		}
		settingsData = data
		res.Settings = true
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, coll := range res.Replaced {
		if _, err := tx.ExecContext(ctx, `DELETE FROM docs WHERE coll=?`, coll); err != nil {
			return nil, err
		}
	}
	if res.Settings {
		if _, err := tx.ExecContext(ctx, `DELETE FROM docs WHERE coll=? AND id=?`, settingsColl, settingsID); err != nil {
			return nil, err
		}
	}
	now := s.stamp()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO docs(coll, id, data, created_at, updated_at) VALUES(?,?,?,?,?)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.coll, r.id, r.data, now, now); err != nil {
			return nil, err
		}
	}
	if res.Settings {
		if _, err := stmt.ExecContext(ctx, settingsColl, settingsID, settingsData, now, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
