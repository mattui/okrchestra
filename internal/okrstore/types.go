package okrstore

// Scope represents the OKR scope level.
type Scope string

const (
	ScopeOrg    Scope = "org"
	ScopeTeam   Scope = "team"
	ScopePerson Scope = "person"
)

// Document is a normalized OKR document loaded from YAML.
type Document struct {
	Scope      Scope
	Objectives []Objective
	Source     string
}

// Objective represents a single objective and its key results.
type Objective struct {
	ID            string
	Objective     string
	OwnerID       string
	Notes         string
	KeyResults    []KeyResult
	SourceFile    string
	DocumentScope Scope
}

// KeyResult captures a single key result.
type KeyResult struct {
	ID          string
	Description string
	OwnerID     string
	MetricKey   string
	Baseline    float64
	Target      float64
	Confidence  float64
	Status      string
	Evidence    []string
	Current     *float64
	LastUpdated string
}

// OrgOKRs groups organization-level objectives.
type OrgOKRs struct {
	Documents []Document
}

// TeamOKRs groups team-level objectives.
type TeamOKRs struct {
	Documents []Document
}

// PersonOKRs groups person-level objectives.
type PersonOKRs struct {
	Documents []Document
}

// ObjectiveRecord maps an objective id to its normalized data and source.
type ObjectiveRecord struct {
	Objective Objective
	Scope     Scope
	Source    string
}

// KeyResultRecord maps a key result id to its normalized data and origin.
type KeyResultRecord struct {
	KeyResult KeyResult
	Objective Objective
	Scope     Scope
	Source    string
}

// Store is the in-memory representation of loaded OKRs.
type Store struct {
	Org OrgOKRs

	Team TeamOKRs

	Person PersonOKRs

	objectives map[string]ObjectiveRecord
	keyResults map[string]KeyResultRecord
}

// ObjectiveLookup returns the objective record for the given id, if present.
func (s *Store) ObjectiveLookup(id string) (ObjectiveRecord, bool) {
	if s == nil {
		return ObjectiveRecord{}, false
	}
	rec, ok := s.objectives[id]
	return rec, ok
}

// KeyResultLookup returns the key result record for the given id, if present.
func (s *Store) KeyResultLookup(id string) (KeyResultRecord, bool) {
	if s == nil {
		return KeyResultRecord{}, false
	}
	rec, ok := s.keyResults[id]
	return rec, ok
}
