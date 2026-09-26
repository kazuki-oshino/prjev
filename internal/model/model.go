package model

import "time"

const Version = "0.1.0"

type Rule struct {
	ID           string            `json:"id" yaml:"id"`
	Title        string            `json:"title" yaml:"title"`
	Tag          string            `json:"tag" yaml:"tag"`
	Priority     string            `json:"priority" yaml:"priority"`
	Instructions string            `json:"instructions" yaml:"instructions"`
	Criteria     map[string]string `json:"criteria" yaml:"criteria"`
	SuggestAt    float64           `json:"suggest_at" yaml:"suggest_at"`
	CandidateAt  float64           `json:"candidate_at" yaml:"candidate_at"`
}
type PR struct {
	URL          string    `json:"url"`
	Repository   string    `json:"repository"`
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Body         string    `json:"body,omitempty"`
	BaseSHA      string    `json:"base_sha"`
	HeadSHA      string    `json:"head_sha"`
	ChangedFiles int       `json:"changed_files"`
	FetchedAt    time.Time `json:"fetched_at"`
	Hash         string    `json:"snapshot_hash"`
}
type Hunk struct {
	ID       string `json:"id"`
	OldStart int    `json:"old_start"`
	OldCount int    `json:"old_count"`
	NewStart int    `json:"new_start"`
	NewCount int    `json:"new_count"`
	Patch    string `json:"-"`
}
type File struct {
	ID           string   `json:"id"`
	Path         string   `json:"path"`
	PreviousPath string   `json:"previous_path,omitempty"`
	Status       string   `json:"status"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
	Patch        string   `json:"-"`
	PatchState   string   `json:"patch_state"`
	Attributes   []string `json:"attributes"`
	Reasons      []string `json:"reasons,omitempty"`
	Hunks        []Hunk   `json:"hunks,omitempty"`
	Units        []string `json:"units,omitempty"`
}
type Unit struct {
	ID                 string   `json:"id"`
	FileID             string   `json:"file_id"`
	Path               string   `json:"path"`
	HunkIDs            []string `json:"hunk_ids"`
	Patch              string   `json:"patch"`
	FileContextPartial bool     `json:"file_context_partial"`
}
type Signal struct {
	UnitID    string   `json:"unit_id"`
	RuleID    string   `json:"rule_id"`
	Value     *float64 `json:"value"`
	State     string   `json:"state"`
	Model     string   `json:"model,omitempty"`
	BatchID   string   `json:"batch_id,omitempty"`
	ErrorCode string   `json:"error_code,omitempty"`
}
type ChecklistItem struct {
	RuleID    string   `json:"rule_id"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	MaxSignal *float64 `json:"max_signal"`
	Targets   []Target `json:"targets"`
	Unknown   bool     `json:"unknown"`
}
type Target struct {
	Path      string   `json:"path"`
	HunkIDs   []string `json:"hunk_ids,omitempty"`
	OldRanges []string `json:"old_ranges,omitempty"`
	NewRanges []string `json:"new_ranges,omitempty"`
}
type FileResult struct {
	Path           string         `json:"path"`
	PreviousPath   string         `json:"previous_path,omitempty"`
	Status         string         `json:"status"`
	Group          string         `json:"group"`
	ContextPartial bool           `json:"file_context_partial"`
	Tags           []string       `json:"tags"`
	Targets        []Target       `json:"targets"`
	Reasons        []string       `json:"reasons,omitempty"`
	Review         ReviewDecision `json:"review"`
}

// ChoiceAnswer retains Jev's confidence, not a probability of bug-free code.
type ChoiceAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}
type ReviewJudgment struct {
	UnitID string `json:"unit_id"`
	ChoiceAnswer
	Target Target `json:"target"`
}
type ReviewDecision struct {
	Level     string           `json:"level"`
	Reason    string           `json:"reason"`
	Judgments []ReviewJudgment `json:"judgments"`
	// Presentation evidence from the branch that selected Level; not another judgment.
	Basis  string   `json:"basis,omitempty"`
	Checks []string `json:"checks,omitempty"`
}
type ReviewPolicy struct {
	Version          int     `json:"version"`
	SkipConfidenceAt float64 `json:"skip_confidence_at"`
}
type Scope struct {
	FetchedFiles  int          `json:"fetched_files"`
	AnalyzedFiles int          `json:"analyzed_files"`
	AnalysisUnits int          `json:"analysis_units"`
	Unanalyzed    []Unanalyzed `json:"unanalyzed"`
	BodyOmitted   bool         `json:"body_omitted"`
}
type Unanalyzed struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
type Metrics struct {
	GitHubMS     int64  `json:"github_ms"`
	JevMS        int64  `json:"jev_ms"`
	LocalMS      int64  `json:"local_ms"`
	TotalMS      int64  `json:"total_ms"`
	HTTPAttempts int    `json:"http_attempts"`
	Usage        *Usage `json:"usage"`
}
type Result struct {
	SchemaVersion int    `json:"schema_version"`
	ToolVersion   string `json:"tool_version"`
	Status        string `json:"status"`
	PR            PR     `json:"pr"`
	Model         struct {
		Requested string   `json:"requested"`
		Actual    []string `json:"actual"`
	} `json:"model"`
	RulesHash        string          `json:"rules_hash"`
	ThresholdProfile []RuleThreshold `json:"threshold_profile"`
	ReviewPolicy     ReviewPolicy    `json:"review_policy"`
	Scope            Scope           `json:"scope"`
	Checklist        []ChecklistItem `json:"checklist"`
	Files            []FileResult    `json:"files"`
	Metrics          Metrics         `json:"metrics"`
	Warnings         []string        `json:"warnings"`
}
type RuleThreshold struct {
	ID          string  `json:"id"`
	SuggestAt   float64 `json:"suggest_at"`
	CandidateAt float64 `json:"candidate_at"`
}
