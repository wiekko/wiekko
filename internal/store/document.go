package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nexmap.net/internal/model"
)

// DocumentFilter controls which documents a list returns.
type DocumentFilter struct {
	State        string // empty = all
	Format       string // empty = all
	ComponentID  string // empty = all
	CapabilityID string // empty = all
	MilestoneID  string // empty = all
	// WithBody includes the body; lists leave it out by default because a
	// body can be large and a list rarely needs it.
	WithBody bool
}

// DocumentSortColumns maps the sort keys callers may ask for onto SQL
// expressions; unknown keys fall back to the default order.
var DocumentSortColumns = map[string]string{
	"identifier": `d.sequence_number`,
	"title":      `LOWER(d.title)`,
	"format":     `d.format`,
	"state":      `CASE d.state WHEN 'current' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END`,
	"revision":   `d.revision`,
	"updated":    `d.updated_at`,
}

const documentDefaultOrder = `d.sequence_number`

// documentLinkTables maps a link target type onto its link table and the
// column naming the target. Only these fixed names ever reach SQL.
var documentLinkTables = map[string][2]string{
	model.DocumentLinkRequirement: {"requirement_documents", "requirement_id"},
	model.DocumentLinkConstraint:  {"constraint_documents", "constraint_id"},
	model.DocumentLinkTestCase:    {"test_case_documents", "test_case_id"},
}

// DocumentStore defines data access operations for documents (FR-50..FR-58).
type DocumentStore interface {
	// Create inserts the document together with its first revision.
	Create(ctx context.Context, d *model.Document, rev *model.DocumentRevision) error
	GetByID(ctx context.Context, id string) (*model.Document, error)
	GetBySequence(ctx context.Context, productID string, seq int) (*model.Document, error)
	NextSequenceNumber(ctx context.Context, productID string) (int, error)
	// Update writes the document's fields. When rev is not nil, the revision
	// is inserted in the same transaction (FR-57).
	Update(ctx context.Context, d *model.Document, rev *model.DocumentRevision) error
	Delete(ctx context.Context, id string) error
	ListPageByProductID(ctx context.Context, productID string, filter DocumentFilter, params ListParams) ([]*model.Document, error)
	CountByProductID(ctx context.Context, productID string, filter DocumentFilter, search string) (int, error)
	// ListRevisions returns a document's revisions, newest first, with empty
	// bodies: each is a full snapshot, so GetRevision loads one when needed.
	ListRevisions(ctx context.Context, documentID string) ([]*model.DocumentRevision, error)
	GetRevision(ctx context.Context, documentID string, revision int) (*model.DocumentRevision, error)
	Link(ctx context.Context, link *model.DocumentLink) error
	Unlink(ctx context.Context, documentID, targetType, targetID string) error
	ListLinks(ctx context.Context, documentID string) ([]*model.DocumentLink, error)
	// ListByTarget returns the documents linked to a requirement, constraint
	// or test case, without their bodies, ordered by identifier.
	ListByTarget(ctx context.Context, targetType, targetID string) ([]*model.Document, error)
}

type documentStore struct {
	db *DB
}

func (s *documentStore) Create(ctx context.Context, d *model.Document, rev *model.DocumentRevision) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO documents (id, product_id, sequence_number, title, body, format, state,
		 superseded_by_id, component_id, capability_id, milestone_id, revision, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProductID, d.SequenceNumber, d.Title, d.Body, d.Format, d.State,
		nullableText(d.SupersededByID), nullableText(d.ComponentID), nullableText(d.CapabilityID),
		nullableText(d.MilestoneID), d.Revision, d.CreatedAt, d.UpdatedAt,
	); err != nil {
		return err
	}
	if err := insertRevision(ctx, tx, rev); err != nil {
		return err
	}
	return tx.Commit()
}

func insertRevision(ctx context.Context, tx *Tx, rev *model.DocumentRevision) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO document_revisions (id, document_id, revision, title, body, format, user_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rev.ID, rev.DocumentID, rev.Revision, rev.Title, rev.Body, rev.Format, rev.UserID, rev.CreatedAt)
	return err
}

func (s *documentStore) GetByID(ctx context.Context, id string) (*model.Document, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+documentColumns(true)+documentFrom+` WHERE d.id = ?`, id)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("document not found: %w", err)
	}
	return d, err
}

func (s *documentStore) GetBySequence(ctx context.Context, productID string, seq int) (*model.Document, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+documentColumns(true)+documentFrom+` WHERE d.product_id = ? AND d.sequence_number = ?`,
		productID, seq)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("document not found: %w", err)
	}
	return d, err
}

// NextSequenceNumber returns the next free document number of the product.
// A concurrent insert can take it first; the unique index then rejects the
// second insert and the caller retries.
func (s *documentStore) NextSequenceNumber(ctx context.Context, productID string) (int, error) {
	var maxSeq sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(sequence_number) FROM documents WHERE product_id = ?`, productID).Scan(&maxSeq); err != nil {
		return 0, err
	}
	if maxSeq.Valid {
		return int(maxSeq.Int64) + 1, nil
	}
	return 1, nil
}

func (s *documentStore) Update(ctx context.Context, d *model.Document, rev *model.DocumentRevision) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`UPDATE documents SET title = ?, body = ?, format = ?, state = ?, superseded_by_id = ?,
		 component_id = ?, capability_id = ?, milestone_id = ?, revision = ?, updated_at = ?
		 WHERE id = ?`,
		d.Title, d.Body, d.Format, d.State, nullableText(d.SupersededByID),
		nullableText(d.ComponentID), nullableText(d.CapabilityID), nullableText(d.MilestoneID),
		d.Revision, d.UpdatedAt, d.ID,
	); err != nil {
		return err
	}
	if rev != nil {
		if err := insertRevision(ctx, tx, rev); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *documentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, id)
	return err
}

func documentListWhere(productID string, filter DocumentFilter, search string) (string, []any) {
	where := ` WHERE d.product_id = ?`
	args := []any{productID}
	for _, f := range []struct{ col, val string }{
		{"d.state", filter.State}, {"d.format", filter.Format}, {"d.component_id", filter.ComponentID},
		{"d.capability_id", filter.CapabilityID}, {"d.milestone_id", filter.MilestoneID},
	} {
		if f.val != "" {
			where += ` AND ` + f.col + ` = ?`
			args = append(args, f.val)
		}
	}
	if search != "" {
		pattern := searchPattern(search)
		where += ` AND (LOWER(d.title) LIKE ? ESCAPE '` + LikeEscape + `'` +
			` OR LOWER(p.document_prefix || '-' || CAST(d.sequence_number AS TEXT)) LIKE ? ESCAPE '` + LikeEscape + `')`
		args = append(args, pattern, pattern)
	}
	return where, args
}

func (s *documentStore) ListPageByProductID(ctx context.Context, productID string, filter DocumentFilter, params ListParams) ([]*model.Document, error) {
	where, args := documentListWhere(productID, filter, params.Search)
	query := `SELECT ` + documentColumns(filter.WithBody) + documentFrom + where +
		orderClause(params, DocumentSortColumns, documentDefaultOrder, `d.id`)
	limit, limitArgs := limitClause(params)
	query += limit
	args = append(args, limitArgs...)
	return s.list(ctx, query, args...)
}

func (s *documentStore) CountByProductID(ctx context.Context, productID string, filter DocumentFilter, search string) (int, error) {
	where, args := documentListWhere(productID, filter, search)
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+documentFrom+where, args...).Scan(&total)
	return total, err
}

func (s *documentStore) ListRevisions(ctx context.Context, documentID string) ([]*model.DocumentRevision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, document_id, revision, title, '', format, user_id, created_at
		 FROM document_revisions WHERE document_id = ? ORDER BY revision DESC`, documentID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var revs []*model.DocumentRevision
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		revs = append(revs, r)
	}
	return revs, rows.Err()
}

func (s *documentStore) GetRevision(ctx context.Context, documentID string, revision int) (*model.DocumentRevision, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, document_id, revision, title, body, format, user_id, created_at
		 FROM document_revisions WHERE document_id = ? AND revision = ?`, documentID, revision)
	r, err := scanRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("document revision not found: %w", err)
	}
	return r, err
}

func scanRevision(sc scanner) (*model.DocumentRevision, error) {
	var r model.DocumentRevision
	if err := sc.Scan(&r.ID, &r.DocumentID, &r.Revision, &r.Title, &r.Body, &r.Format, &r.UserID, &r.CreatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

func linkTable(targetType string) (table, column string, err error) {
	t, ok := documentLinkTables[targetType]
	if !ok {
		return "", "", fmt.Errorf("documents cannot be linked to %q", targetType)
	}
	return t[0], t[1], nil
}

func (s *documentStore) Link(ctx context.Context, link *model.DocumentLink) error {
	table, column, err := linkTable(link.TargetType)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO `+table+` (id, `+column+`, document_id, created_at) VALUES (?, ?, ?, ?)`,
		link.ID, link.TargetID, link.DocumentID, link.CreatedAt)
	return err
}

func (s *documentStore) Unlink(ctx context.Context, documentID, targetType, targetID string) error {
	table, column, err := linkTable(targetType)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM `+table+` WHERE document_id = ? AND `+column+` = ?`, documentID, targetID)
	return err
}

func (s *documentStore) ListLinks(ctx context.Context, documentID string) ([]*model.DocumentLink, error) {
	var links []*model.DocumentLink
	for _, target := range []string{model.DocumentLinkRequirement, model.DocumentLinkConstraint, model.DocumentLinkTestCase} {
		table, column, _ := linkTable(target)
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, document_id, `+column+`, created_at FROM `+table+
				` WHERE document_id = ? ORDER BY created_at, id`, documentID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			l := model.DocumentLink{TargetType: target}
			if err := rows.Scan(&l.ID, &l.DocumentID, &l.TargetID, &l.CreatedAt); err != nil {
				closeRows(rows)
				return nil, err
			}
			links = append(links, &l)
		}
		err = rows.Err()
		closeRows(rows)
		if err != nil {
			return nil, err
		}
	}
	return links, nil
}

func (s *documentStore) ListByTarget(ctx context.Context, targetType, targetID string) ([]*model.Document, error) {
	table, column, err := linkTable(targetType)
	if err != nil {
		return nil, err
	}
	return s.list(ctx,
		`SELECT `+documentColumns(false)+documentFrom+
			` JOIN `+table+` l ON l.document_id = d.id WHERE l.`+column+` = ? ORDER BY d.sequence_number`,
		targetID)
}

func (s *documentStore) list(ctx context.Context, query string, args ...any) ([]*model.Document, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var docs []*model.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// documentFrom joins the product for the identifier prefix, so every read
// yields the identifier with the prefix currently configured (FR-56).
const documentFrom = ` FROM documents d JOIN products p ON d.product_id = p.id`

// documentColumns lists the columns scanDocument reads; without the body, an
// empty string stands in for it.
func documentColumns(withBody bool) string {
	body := `''`
	if withBody {
		body = `d.body`
	}
	return `d.id, d.product_id, d.sequence_number, p.document_prefix, d.title, ` + body + `, d.format, d.state,
	d.superseded_by_id, d.component_id, d.capability_id, d.milestone_id, d.revision, d.created_at, d.updated_at`
}

func scanDocument(sc scanner) (*model.Document, error) {
	var d model.Document
	var prefix string
	var supersededBy, componentID, capabilityID, milestoneID sql.NullString
	if err := sc.Scan(&d.ID, &d.ProductID, &d.SequenceNumber, &prefix, &d.Title, &d.Body, &d.Format, &d.State,
		&supersededBy, &componentID, &capabilityID, &milestoneID, &d.Revision, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.SupersededByID = supersededBy.String
	d.ComponentID = componentID.String
	d.CapabilityID = capabilityID.String
	d.MilestoneID = milestoneID.String
	d.Identifier = model.EntityIdentifier(prefix, d.SequenceNumber)
	return &d, nil
}
