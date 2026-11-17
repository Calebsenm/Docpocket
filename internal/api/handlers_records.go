package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/docpocket/docpocket/internal/auth"
	"github.com/docpocket/docpocket/internal/collections"
	"github.com/docpocket/docpocket/internal/query"
	"github.com/docpocket/docpocket/internal/realtime"
	"github.com/docpocket/docpocket/internal/rules"
	"github.com/docpocket/docpocket/internal/storage"
)

func (s *Server) handleListRecords(w http.ResponseWriter, r *http.Request) {
	s.listRecords(w, r, false)
}

func (s *Server) handleAdminListRecords(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	s.listRecords(w, r, true)
}

func (s *Server) listRecords(w http.ResponseWriter, r *http.Request, admin bool) {
	col := r.PathValue("col")
	id := identityFrom(r)
	if !admin && id != nil && id.IsAdmin() {
		admin = true
	}
	var def *collections.CollectionDef
	if err := s.store.View(func(tx storage.ReadTx) error {
		var err error
		def, err = tx.Collection(col)
		return err
	}); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if !admin {
		if _, ok := def.RuleSet("list"); !ok {
			writeError(w, http.StatusForbidden, "forbidden", "list access is not configured for this collection", nil)
			return
		}
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	fields, err := parseFields(r.URL.Query().Get("fields"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_fields", err.Error(), nil)
		return
	}
	var authMap map[string]any
	if id != nil {
		authMap = id.AuthMap()
	}
	opts := query.ListOptions{
		Filter: r.URL.Query().Get("filter"),
		Sort:   r.URL.Query().Get("sort"),
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
		Fields: fields,
		Auth:   authMap,
	}
	if !admin {
		queryURL := r.URL.Query()
		queryMap := queryValues(queryURL)
		opts.Authorize = func(doc collections.Document) (bool, error) {
			return s.authorizeQuery(def, "list", id, authMap, doc, nil, queryMap)
		}
	}
	res, err := s.engine.List(r.Context(), col, opts)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	items := make([]collections.Document, len(res.Items))
	for i, doc := range res.Items {
		items[i] = sanitizeRecord(def, doc)
	}
	resp := listResponse{Items: items}
	if res.NextCursor != "" {
		c := res.NextCursor
		resp.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, resp)
}

type listResponse struct {
	Items      []collections.Document `json:"items"`
	NextCursor *string                `json:"nextCursor"`
}

func (s *Server) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	col, docID := r.PathValue("col"), r.PathValue("id")
	var (
		def *collections.CollectionDef
		doc collections.Document
	)
	if err := s.store.View(func(tx storage.ReadTx) error {
		var err error
		def, err = tx.Collection(col)
		if err != nil {
			return err
		}
		var ok bool
		doc, ok, err = tx.GetDoc(col, docID)
		if err != nil {
			return err
		}
		if !ok {
			return storage.ErrNotFound
		}
		return nil
	}); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	allowed, err := s.authorize(def, "view", identityFrom(r), doc, nil, r.URL.Query())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot view this record", nil)
		return
	}
	writeJSON(w, http.StatusOK, sanitizeRecord(def, doc))
}

func (s *Server) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	col := r.PathValue("col")
	var def *collections.CollectionDef
	if err := s.store.View(func(tx storage.ReadTx) error {
		var err error
		def, err = tx.Collection(col)
		return err
	}); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	var body collections.Document
	if err := s.decodeJSON(w, r, &body); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if body == nil {
		body = collections.Document{}
	}
	id := identityFrom(r)
	if def.Type == collections.TypeAuth {
		if id == nil || !id.IsAdmin() {
			writeError(w, http.StatusForbidden, "use_auth_endpoint", "create auth users through auth-register", nil)
			return
		}
		// Superusers can provision users directly. The password is hashed by the
		// auth service and never stored in the document body.
		email, _ := body[collections.FieldEmail].(string)
		password, _ := body["password"].(string)
		delete(body, "password")
		session, err := s.auth.Register(col, email, password, body)
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
		record := session.Record
		if record == nil {
			record = collections.Document{}
		}
		s.hub.Publish([]realtime.Event{{
			Action:     "create",
			Collection: col,
			ID:         record.ID(),
			Record:     record.Clone(),
		}})
		writeJSON(w, http.StatusCreated, sanitizeRecord(def, record))
		return
	}
	allowed, err := s.authorize(def, "create", id, nil, body, r.URL.Query())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot create records in this collection", nil)
		return
	}
	if s.hooks.OnRecordBeforeCreate != nil {
		if err := s.hooks.OnRecordBeforeCreate(col, body, id); err != nil {
			writeError(w, http.StatusBadRequest, "hook_rejected", err.Error(), nil)
			return
		}
	}
	if err := s.store.Update(func(tx storage.WriteTx) error { return tx.PutDoc(col, body) }); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if s.hooks.OnRecordAfterCreate != nil {
		s.hooks.OnRecordAfterCreate(col, body, id)
	}
	s.hub.Publish([]realtime.Event{{
		Action:     "create",
		Collection: col,
		ID:         body.ID(),
		Record:     body.Clone(),
	}})
	writeJSON(w, http.StatusCreated, sanitizeRecord(def, body))
}

func (s *Server) handleUpdateRecord(w http.ResponseWriter, r *http.Request) {
	s.updateRecord(w, r, true)
}

func (s *Server) handleReplaceRecord(w http.ResponseWriter, r *http.Request) {
	s.updateRecord(w, r, false)
}

func (s *Server) updateRecord(w http.ResponseWriter, r *http.Request, patch bool) {
	col, docID := r.PathValue("col"), r.PathValue("id")
	var body collections.Document
	if err := s.decodeJSON(w, r, &body); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if body == nil {
		body = collections.Document{}
	}
	id := identityFrom(r)
	if id == nil || !id.IsAdmin() {
		if err := protectAuthFields(body); err != nil {
			writeError(w, http.StatusForbidden, "forbidden", err.Error(), nil)
			return
		}
	}
	var (
		def   *collections.CollectionDef
		old   collections.Document
		saved collections.Document
	)
	err := s.store.Update(func(tx storage.WriteTx) error {
		var err error
		def, err = tx.Collection(col)
		if err != nil {
			return err
		}
		var ok bool
		old, ok, err = tx.GetDoc(col, docID)
		if err != nil {
			return err
		}
		if !ok {
			return storage.ErrNotFound
		}
		if body.ID() != "" && body.ID() != docID {
			return &collections.ValidationError{Field: "id", Message: "id cannot be changed"}
		}
		if patch {
			if _, ok := body[collections.FieldCreated]; ok {
				return &collections.ValidationError{Field: "created", Message: "created is read-only"}
			}
		}
		allowed, err := s.authorize(def, "update", id, old, body, r.URL.Query())
		if err != nil {
			return err
		}
		if !allowed {
			return errForbidden("you cannot update this record")
		}
		next := old.Clone()
		if patch {
			collections.MergePatch(next, body)
		} else {
			next = body.Clone()
		}
		next[collections.FieldID] = docID
		next[collections.FieldCreated] = old[collections.FieldCreated]
		// Credential fields are managed by the auth service and never accepted
		// from a generic record update.
		for _, hidden := range []string{collections.FieldPasswordHash, collections.FieldTokenKey} {
			if v, ok := old[hidden]; ok {
				next[hidden] = v
			} else {
				delete(next, hidden)
			}
		}
		if def.Type == collections.TypeAuth && !id.IsAdmin() {
			next[collections.FieldEmail] = old[collections.FieldEmail]
			next[collections.FieldVerified] = old[collections.FieldVerified]
		}
		if s.hooks.OnRecordBeforeUpdate != nil {
			if err := s.hooks.OnRecordBeforeUpdate(col, next, old, id); err != nil {
				return err
			}
		}
		if err := tx.PutDoc(col, next); err != nil {
			return err
		}
		saved = next
		return nil
	})
	if err != nil {
		if errors.Is(err, errForbiddenSentinel) {
			writeError(w, http.StatusForbidden, "forbidden", "you cannot update this record", nil)
			return
		}
		s.writeServiceError(w, r, err)
		return
	}
	if s.hooks.OnRecordAfterUpdate != nil {
		s.hooks.OnRecordAfterUpdate(col, saved, old, id)
	}
	s.hub.Publish([]realtime.Event{{
		Action:     "update",
		Collection: col,
		ID:         docID,
		Record:     saved.Clone(),
		Old:        old.Clone(),
	}})
	writeJSON(w, http.StatusOK, sanitizeRecord(def, saved))
}

func (s *Server) handleDeleteRecord(w http.ResponseWriter, r *http.Request) {
	col, docID := r.PathValue("col"), r.PathValue("id")
	id := identityFrom(r)
	var (
		def *collections.CollectionDef
		old collections.Document
	)
	err := s.store.Update(func(tx storage.WriteTx) error {
		var err error
		def, err = tx.Collection(col)
		if err != nil {
			return err
		}
		var ok bool
		old, ok, err = tx.GetDoc(col, docID)
		if err != nil {
			return err
		}
		if !ok {
			return storage.ErrNotFound
		}
		allowed, err := s.authorize(def, "delete", id, old, nil, r.URL.Query())
		if err != nil {
			return err
		}
		if !allowed {
			return errForbidden("you cannot delete this record")
		}
		if s.hooks.OnRecordBeforeDelete != nil {
			if err := s.hooks.OnRecordBeforeDelete(col, old, id); err != nil {
				return err
			}
		}
		return tx.DeleteDoc(col, docID)
	})
	if err != nil {
		if errors.Is(err, errForbiddenSentinel) {
			writeError(w, http.StatusForbidden, "forbidden", "you cannot delete this record", nil)
			return
		}
		s.writeServiceError(w, r, err)
		return
	}
	if s.hooks.OnRecordAfterDelete != nil {
		s.hooks.OnRecordAfterDelete(col, old, id)
	}
	s.hub.Publish([]realtime.Event{{
		Action:     "delete",
		Collection: col,
		ID:         docID,
		Old:        old.Clone(),
	}})
	// Files are deleted after the database commit.
	go func() {
		if err := s.files.DeleteDocument(col, docID); err != nil {
			s.logger.Warn("could not delete document files", "collection", col, "id", docID, "error", err)
		}
	}()
	w.WriteHeader(http.StatusNoContent)
}

// ---- batch ----

type batchRequest struct {
	Operations []batchOperation `json:"operations"`
}

type batchOperation struct {
	Method     string               `json:"method"`
	Collection string               `json:"collection"`
	ID         string               `json:"id,omitempty"`
	Record     collections.Document `json:"record,omitempty"`
}

type batchResult struct {
	Method     string               `json:"method"`
	Collection string               `json:"collection"`
	Record     collections.Document `json:"record,omitempty"`
	Deleted    bool                 `json:"deleted,omitempty"`
	def        *collections.CollectionDef
}

func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := s.decodeJSON(w, r, &req); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if len(req.Operations) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_batch", "at least one operation is required", nil)
		return
	}
	if len(req.Operations) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_batch", "at most 100 operations are allowed", nil)
		return
	}
	id := identityFrom(r)
	results := make([]batchResult, 0, len(req.Operations))
	var events []realtime.Event
	err := s.store.Update(func(tx storage.WriteTx) error {
		for i, op := range req.Operations {
			method := strings.ToUpper(op.Method)
			if op.Collection == "" {
				return fmt.Errorf("operation %d: collection is required", i)
			}
			def, err := tx.Collection(op.Collection)
			if err != nil {
				return fmt.Errorf("operation %d: %w", i, err)
			}
			if method == http.MethodPost && def.Type == collections.TypeAuth {
				return errForbidden("operation " + fmt.Sprint(i) + ": use auth-register for auth collections")
			}
			switch method {
			case http.MethodPost:
				body := op.Record
				if body == nil {
					body = collections.Document{}
				}
				if id == nil || !id.IsAdmin() {
					if err := protectAuthFields(body); err != nil {
						return errForbidden(err.Error())
					}
				}
				allowed, err := s.authorize(def, "create", id, nil, body, nil)
				if err != nil {
					return err
				}
				if !allowed {
					return errForbidden("operation " + fmt.Sprint(i) + ": create not allowed")
				}
				if err := tx.PutDoc(op.Collection, body); err != nil {
					return fmt.Errorf("operation %d: %w", i, err)
				}
				results = append(results, batchResult{Method: method, Collection: op.Collection, Record: body.Clone(), def: def})
				events = append(events, realtime.Event{Action: "create", Collection: op.Collection, ID: body.ID(), Record: body.Clone()})
			case http.MethodPatch, http.MethodPut:
				docID := op.ID
				if docID == "" && op.Record != nil {
					docID = op.Record.ID()
				}
				if docID == "" {
					return fmt.Errorf("operation %d: id is required", i)
				}
				old, ok, err := tx.GetDoc(op.Collection, docID)
				if err != nil {
					return fmt.Errorf("operation %d: %w", i, err)
				}
				if !ok {
					return fmt.Errorf("operation %d: %w", i, storage.ErrNotFound)
				}
				body := op.Record
				if body == nil {
					body = collections.Document{}
				}
				if id == nil || !id.IsAdmin() {
					if err := protectAuthFields(body); err != nil {
						return errForbidden(err.Error())
					}
				}
				allowed, err := s.authorize(def, "update", id, old, body, nil)
				if err != nil {
					return err
				}
				if !allowed {
					return errForbidden("operation " + fmt.Sprint(i) + ": update not allowed")
				}
				next := old.Clone()
				if method == http.MethodPatch {
					collections.MergePatch(next, body)
				} else {
					next = body.Clone()
				}
				next[collections.FieldID] = docID
				next[collections.FieldCreated] = old[collections.FieldCreated]
				for _, hidden := range []string{collections.FieldPasswordHash, collections.FieldTokenKey} {
					if v, ok := old[hidden]; ok {
						next[hidden] = v
					} else {
						delete(next, hidden)
					}
				}
				if def.Type == collections.TypeAuth && !id.IsAdmin() {
					next[collections.FieldEmail] = old[collections.FieldEmail]
					next[collections.FieldVerified] = old[collections.FieldVerified]
				}
				if err := tx.PutDoc(op.Collection, next); err != nil {
					return fmt.Errorf("operation %d: %w", i, err)
				}
				results = append(results, batchResult{Method: method, Collection: op.Collection, Record: next.Clone(), def: def})
				events = append(events, realtime.Event{Action: "update", Collection: op.Collection, ID: docID, Record: next.Clone(), Old: old.Clone()})
			case http.MethodDelete:
				docID := op.ID
				if docID == "" && op.Record != nil {
					docID = op.Record.ID()
				}
				if docID == "" {
					return fmt.Errorf("operation %d: id is required", i)
				}
				old, ok, err := tx.GetDoc(op.Collection, docID)
				if err != nil {
					return fmt.Errorf("operation %d: %w", i, err)
				}
				if !ok {
					return fmt.Errorf("operation %d: %w", i, storage.ErrNotFound)
				}
				allowed, err := s.authorize(def, "delete", id, old, nil, nil)
				if err != nil {
					return err
				}
				if !allowed {
					return errForbidden("operation " + fmt.Sprint(i) + ": delete not allowed")
				}
				if err := tx.DeleteDoc(op.Collection, docID); err != nil {
					return fmt.Errorf("operation %d: %w", i, err)
				}
				results = append(results, batchResult{Method: method, Collection: op.Collection, Deleted: true, Record: old.Clone(), def: def})
				events = append(events, realtime.Event{Action: "delete", Collection: op.Collection, ID: docID, Old: old.Clone()})
			default:
				return fmt.Errorf("operation %d: unsupported method %q", i, op.Method)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errForbiddenSentinel) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error(), nil)
			return
		}
		s.writeServiceError(w, r, err)
		return
	}
	s.hub.Publish(events)
	for i := range results {
		if results[i].Record != nil {
			results[i].Record = sanitizeRecord(results[i].def, results[i].Record)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// ---- helpers ----

func (s *Server) authorize(def *collections.CollectionDef, op string, id *auth.Identity, resource, body collections.Document, q url.Values) (bool, error) {
	var queryMap map[string]any
	if q != nil {
		queryMap = queryValues(q)
	}
	return s.authorizeQuery(def, op, id, nil, resource, body, queryMap)
}

// authorizeQuery is authorize with precomputed identity and query values. The
// list handler uses it once per document, with query values shared.
func (s *Server) authorizeQuery(def *collections.CollectionDef, op string, id *auth.Identity, ident map[string]any, resource, body collections.Document, queryMap map[string]any) (bool, error) {
	if ident == nil && id != nil {
		ident = id.AuthMap()
	}
	var res map[string]any
	if resource != nil {
		res = map[string]any(resource.SanitizeView())
	}
	var b map[string]any
	if body != nil {
		b = map[string]any(body.SanitizeView())
	}
	return s.rules.Allowed(def, op, id != nil && id.IsAdmin(), rules.Context{
		Operation: op,
		Identity:  ident,
		Resource:  res,
		Body:      b,
		Query:     queryMap,
	})
}

var errForbiddenSentinel = errors.New("forbidden")

func errForbidden(msg string) error { return fmt.Errorf("%w: %s", errForbiddenSentinel, msg) }

func protectAuthFields(body collections.Document) error {
	for _, key := range []string{collections.FieldPasswordHash, collections.FieldTokenKey} {
		if _, ok := body[key]; ok {
			return fmt.Errorf("%s is managed by the auth service", key)
		}
	}
	return nil
}
