package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// credentialLine is one accounts file row:
//
//	{email}----{password}----{sso_token}
//
// Password and SSO are kept only in process memory for matching / future mint flows.
// They are never written to results.json and never returned to the browser.
type credentialLine struct {
	Email    string
	Password string
	SSO      string
	LineNo   int
}

type credentialStore struct {
	mu        sync.Mutex
	byEmail   map[string]credentialLine // exact email -> latest line
	order     []string                  // upload order of unique emails
	uploaded  int                       // raw accepted lines (including duplicate emails)
	skipped   int                       // malformed / incomplete lines
	uploadedAt time.Time
}

var credentials = &credentialStore{
	byEmail: map[string]credentialLine{},
}

type credentialSummary struct {
	Uploaded   int    `json:"uploaded"`
	Unique     int    `json:"unique"`
	Skipped    int    `json:"skipped"`
	Matched    int    `json:"matched"`
	Eligible   int    `json:"eligible"`
	Unmatched  int    `json:"unmatched"`
	UploadedAt string `json:"uploaded_at,omitempty"`
	HasData    bool   `json:"has_data"`
}

func parseAccountsContent(content string) (accepted []credentialLine, skipped int) {
	lines := strings.Split(content, "\n")
	accepted = make([]credentialLine, 0, len(lines))
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		// Support both "----" register-machine format and occasional fullwidth dashes.
		s = strings.ReplaceAll(s, "——", "----")
		parts := strings.Split(s, "----")
		if len(parts) < 2 {
			skipped++
			continue
		}
		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])
		sso := ""
		if len(parts) > 2 {
			sso = strings.TrimSpace(parts[2])
		}
		if email == "" || password == "" {
			skipped++
			continue
		}
		// Exact email key — do not lower-case (user-requested exact CPA email match).
		accepted = append(accepted, credentialLine{
			Email:    email,
			Password: password,
			SSO:      sso,
			LineNo:   i + 1,
		})
	}
	return accepted, skipped
}

func (s *credentialStore) replaceFromContent(content string) (credentialSummary, error) {
	accepted, skipped := parseAccountsContent(content)
	if len(accepted) == 0 {
		return credentialSummary{}, fmt.Errorf("未解析到有效账号行（格式：邮箱----密码----sso）")
	}
	byEmail := make(map[string]credentialLine, len(accepted))
	order := make([]string, 0, len(accepted))
	for _, item := range accepted {
		if _, exists := byEmail[item.Email]; !exists {
			order = append(order, item.Email)
		}
		byEmail[item.Email] = item // last line wins for duplicates
	}
	s.mu.Lock()
	s.byEmail = byEmail
	s.order = order
	s.uploaded = len(accepted)
	s.skipped = skipped
	s.uploadedAt = time.Now()
	s.mu.Unlock()
	return s.summaryAgainst(engine.snapshot(true).Results), nil
}

func (s *credentialStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byEmail = map[string]credentialLine{}
	s.order = nil
	s.uploaded = 0
	s.skipped = 0
	s.uploadedAt = time.Time{}
}

func (s *credentialStore) hasData() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byEmail) > 0
}

func (s *credentialStore) lookupExact(email string) (credentialLine, bool) {
	email = strings.TrimSpace(email)
	if email == "" {
		return credentialLine{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.byEmail[email]
	return item, ok
}

// matchResultEmail reports whether a result row matches an uploaded credential email.
// Matching is exact on CPA email; when email is empty, xai-<email>.json is accepted.
func matchResultEmail(result accountResult, email string) bool {
	email = strings.TrimSpace(email)
	if email == "" {
		return false
	}
	if strings.TrimSpace(result.Email) == email {
		return true
	}
	if strings.TrimSpace(result.Name) == email {
		return true
	}
	fileName := strings.TrimSpace(result.FileName)
	if fileName == "" {
		fileName = strings.TrimSpace(result.Name)
	}
	return fileName == "xai-"+email+".json"
}

func resultCredentialEmail(result accountResult) string {
	if email := strings.TrimSpace(result.Email); email != "" {
		return email
	}
	if name := strings.TrimSpace(result.Name); strings.Contains(name, "@") {
		return name
	}
	fileName := strings.TrimSpace(result.FileName)
	if fileName == "" {
		fileName = strings.TrimSpace(result.Name)
	}
	fileName = strings.TrimSuffix(fileName, ".json")
	if strings.HasPrefix(fileName, "xai-") {
		return strings.TrimPrefix(fileName, "xai-")
	}
	return ""
}

func isReauthEligible(result accountResult) bool {
	if strings.TrimSpace(result.Classification) == "reauth" {
		return true
	}
	if result.HTTPStatus == 403 && strings.TrimSpace(result.Classification) == "permission_denied" {
		return true
	}
	if result.HTTPStatus == 401 {
		return true
	}
	return false
}

func (s *credentialStore) matchedEligible(results []accountResult) []accountResult {
	s.mu.Lock()
	byEmail := make(map[string]credentialLine, len(s.byEmail))
	for k, v := range s.byEmail {
		byEmail[k] = v
	}
	s.mu.Unlock()
	if len(byEmail) == 0 {
		return nil
	}
	out := make([]accountResult, 0)
	seen := map[string]struct{}{}
	for _, item := range results {
		if !isReauthEligible(item) {
			continue
		}
		email := resultCredentialEmail(item)
		if email == "" {
			continue
		}
		if _, ok := byEmail[email]; !ok {
			// Exact match only; also try direct file-name style keys already in map.
			matched := false
			for upEmail := range byEmail {
				if matchResultEmail(item, upEmail) {
					email = upEmail
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		} else if !matchResultEmail(item, email) {
			// Defensive: ensure the resolved email really matches this row.
			if !matchResultEmail(item, email) {
				continue
			}
		}
		key := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, email)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (s *credentialStore) summaryAgainst(results []accountResult) credentialSummary {
	s.mu.Lock()
	summary := credentialSummary{
		Uploaded: s.uploaded,
		Unique:   len(s.byEmail),
		Skipped:  s.skipped,
		HasData:  len(s.byEmail) > 0,
	}
	if !s.uploadedAt.IsZero() {
		summary.UploadedAt = s.uploadedAt.Format(time.RFC3339)
	}
	emails := make([]string, 0, len(s.byEmail))
	for email := range s.byEmail {
		emails = append(emails, email)
	}
	s.mu.Unlock()

	matchedEmails := map[string]struct{}{}
	eligibleEmails := map[string]struct{}{}
	for _, item := range results {
		for _, email := range emails {
			if !matchResultEmail(item, email) {
				continue
			}
			matchedEmails[email] = struct{}{}
			if isReauthEligible(item) {
				eligibleEmails[email] = struct{}{}
			}
		}
	}
	summary.Matched = len(matchedEmails)
	summary.Eligible = len(eligibleEmails)
	if summary.Unique > summary.Matched {
		summary.Unmatched = summary.Unique - summary.Matched
	}
	return summary
}
