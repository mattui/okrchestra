package okrstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LoadFromDir loads and validates all OKR YAML files from the provided directory.
func LoadFromDir(okrsDir string) (*Store, error) {
	if okrsDir == "" {
		okrsDir = "okrs"
	}

	files, err := filepath.Glob(filepath.Join(okrsDir, "*.yml"))
	if err != nil {
		return nil, fmt.Errorf("scan okr dir: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no OKR YAML files found in %s", okrsDir)
	}
	sort.Strings(files)

	var docs []Document
	var vErrs ValidationErrors

	for _, path := range files {
		base := filepath.Base(path)
		if base == "permissions.yml" {
			// handled by permissions loader
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		doc, parseErr := ParseAndValidateDocument(data, path)
		if parseErr != nil {
			if ve, ok := parseErr.(ValidationErrors); ok {
				vErrs = append(vErrs, ve...)
				continue
			}
			return nil, parseErr
		}
		docs = append(docs, doc)
	}

	if len(vErrs) > 0 {
		return nil, vErrs
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no OKR documents found in %s", okrsDir)
	}

	duplicateErrs := validateCrossDocumentUniqueness(docs)
	if len(duplicateErrs) > 0 {
		return nil, duplicateErrs
	}

	return buildStore(docs), nil
}

func validateCrossDocumentUniqueness(docs []Document) ValidationErrors {
	var errs ValidationErrors

	objByScope := make(map[Scope]map[string]struct{})
	type krOrigin struct {
		scope Scope
		file  string
		objID string
	}
	krSeen := make(map[string]krOrigin)

	for _, doc := range docs {
		if _, ok := objByScope[doc.Scope]; !ok {
			objByScope[doc.Scope] = make(map[string]struct{})
		}

		for objIdx, obj := range doc.Objectives {
			if obj.ID != "" {
				if _, exists := objByScope[doc.Scope][obj.ID]; exists {
					errs = append(errs, ValidationError{
						File:    doc.Source,
						Field:   fmt.Sprintf("objectives[%d].objective_id", objIdx),
						Message: fmt.Sprintf("objective_id %q duplicates another in scope %s", obj.ID, doc.Scope),
					})
				} else {
					objByScope[doc.Scope][obj.ID] = struct{}{}
				}
			}

			for krIdx, kr := range obj.KeyResults {
				if kr.ID == "" {
					continue
				}
				if origin, exists := krSeen[kr.ID]; exists {
					errs = append(errs, ValidationError{
						File:    doc.Source,
						Field:   fmt.Sprintf("objectives[%d].key_results[%d].kr_id", objIdx, krIdx),
						Message: fmt.Sprintf("kr_id %q already defined in %s (%s objective %s)", kr.ID, origin.file, origin.scope, origin.objID),
					})
					continue
				}
				krSeen[kr.ID] = krOrigin{
					scope: doc.Scope,
					file:  doc.Source,
					objID: obj.ID,
				}
			}
		}
	}

	return errs
}

func buildStore(docs []Document) *Store {
	store := &Store{
		Org:        OrgOKRs{},
		Team:       TeamOKRs{},
		Person:     PersonOKRs{},
		objectives: make(map[string]ObjectiveRecord),
		keyResults: make(map[string]KeyResultRecord),
	}

	for _, doc := range docs {
		switch doc.Scope {
		case ScopeOrg:
			store.Org.Documents = append(store.Org.Documents, doc)
		case ScopeTeam:
			store.Team.Documents = append(store.Team.Documents, doc)
		case ScopePerson:
			store.Person.Documents = append(store.Person.Documents, doc)
		default:
			// already validated; defensive only
			continue
		}

		for _, obj := range doc.Objectives {
			objCopy := obj
			objCopy.SourceFile = doc.Source
			objCopy.DocumentScope = doc.Scope

			objRec := ObjectiveRecord{
				Objective: objCopy,
				Scope:     doc.Scope,
				Source:    doc.Source,
			}
			store.objectives[obj.ID] = objRec

			for _, kr := range obj.KeyResults {
				krRec := KeyResultRecord{
					KeyResult: kr,
					Objective: objCopy,
					Scope:     doc.Scope,
					Source:    doc.Source,
				}
				store.keyResults[kr.ID] = krRec
			}
		}
	}

	return store
}

// ListObjectiveIDs returns all objective ids by scope.
func (s *Store) ListObjectiveIDs() map[Scope][]string {
	result := map[Scope][]string{
		ScopeOrg:    {},
		ScopeTeam:   {},
		ScopePerson: {},
	}
	for _, rec := range s.objectives {
		result[rec.Scope] = append(result[rec.Scope], rec.Objective.ID)
	}
	for scope, ids := range result {
		sort.Strings(ids)
		result[scope] = ids
	}
	return result
}

// String scopes for friendly messages.
func (s Scope) String() string {
	return string(s)
}
