package api

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
	"github.com/iLLeniumStudios/protem/internal/audit"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

func paginate[T any](q url.Values, items []T) ([]T, int, error) {
	limit, offset := defaultPageSize, 0
	if q.Has("limit") {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n < 1 || n > maxPageSize {
			return nil, 0, fmt.Errorf("limit must be an integer from 1 to %d", maxPageSize)
		}
		limit = n
	}
	if q.Has("offset") {
		n, err := strconv.Atoi(q.Get("offset"))
		if err != nil || n < 0 {
			return nil, 0, errors.New("offset must be a non-negative integer")
		}
		offset = n
	}
	start := min(offset, len(items))
	page := items[start:min(start+limit, len(items))]
	if page == nil {
		page = []T{}
	}
	return page, len(items), nil
}

func boolFilter(q url.Values, name string) (bool, error) {
	switch q.Get(name) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func choiceFilter(q url.Values, name string, allowed ...string) (string, error) {
	if v := q.Get(name); v == "" || slices.Contains(allowed, v) {
		return v, nil
	}
	return "", fmt.Errorf("%s must be one of %s", name, strings.Join(allowed, ", "))
}

func auditRows(req *protemv1alpha1.AccessRequest, grant *protemv1alpha1.AccessGrant, now time.Time) []AuditItem {
	events := audit.Events(req, grant, now)
	rows := make([]AuditItem, len(events))
	for i, e := range events {
		rows[i] = AuditItem(e)
	}
	return rows
}

func auditSequence(row AuditItem) int {
	n, _ := strconv.Atoi(row.ID[strings.LastIndexByte(row.ID, '#')+1:])
	return n
}

func filterAudit(rows []AuditItem, q url.Values) ([]AuditItem, error) {
	order, err := choiceFilter(q, "order", "asc", "desc")
	if err != nil {
		return nil, err
	}
	event, cluster, text := q.Get("event"), q.Get("cluster"), strings.ToLower(q.Get("q"))
	matchesText := func(r AuditItem) bool {
		return slices.ContainsFunc([]string{r.Requester, r.Actor, r.Profile, r.Ticket, r.Reason, r.Comment}, func(field string) bool {
			return strings.Contains(strings.ToLower(field), text)
		})
	}
	out := make([]AuditItem, 0, len(rows))
	for _, r := range rows {
		if (event == "" || r.EventType == event) && (cluster == "" || slices.Contains(r.Clusters, cluster)) && (text == "" || matchesText(r)) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b AuditItem) int {
		c := cmp.Or(
			a.Timestamp.Compare(b.Timestamp),
			cmp.Compare(a.Namespace, b.Namespace),
			cmp.Compare(a.Request, b.Request),
			cmp.Compare(auditSequence(a), auditSequence(b)),
		)
		if order == "asc" {
			return c
		}
		return -c
	})
	return out, nil
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx, q := r.Context(), r.URL.Query()
	var requests protemv1alpha1.AccessRequestList
	var grants protemv1alpha1.AccessGrantList
	if err := errors.Join(s.Client.List(ctx, &requests), s.Client.List(ctx, &grants)); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	hasGlobalRead, _ := s.checkSAR(ctx, user, "get", "accessrequests", "", "")

	slices.SortFunc(grants.Items, func(a, b protemv1alpha1.AccessGrant) int { return byName(&a, &b) })
	grantOf := map[string]*protemv1alpha1.AccessGrant{}
	for i := range grants.Items {
		g := &grants.Items[i]
		if key := g.Namespace + "/" + g.Spec.RequestRef; grantOf[key] == nil {
			grantOf[key] = g
		}
	}

	person, now := toPerson(user), time.Now()
	var rows []AuditItem
	for i := range requests.Items {
		req := &requests.Items[i]
		if hasGlobalRead || approval.CanView(person, req) {
			rows = append(rows, auditRows(req, grantOf[req.Namespace+"/"+req.Name], now)...)
		}
	}
	rows, err := filterAudit(rows, q)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: err.Error()})
		return
	}
	page, total, err := paginate(q, rows)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{errorKey: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, AuditPage{Items: page, Total: total})
}
