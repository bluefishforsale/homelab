package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	BoardSize          = 12
	VoteMajorityPct    = 70 // 70% required to pass (9 of 12)
	VotesRequiredToPass = 9  // ceil(12 * 0.70)
)

// VoteType represents different types of votes
type VoteType string

const (
	VoteApprove  VoteType = "approve"
	VoteReject   VoteType = "reject"
	VoteAbstain  VoteType = "abstain"
	VoteDefer    VoteType = "defer"
)

// BoardMemberID identifies a board member
type BoardMemberID string

// Google-inspired board member IDs
const (
	MemberChair        BoardMemberID = "chair"           // Board Chair - governance focus
	MemberFounder1     BoardMemberID = "founder_tech"    // Technical founder
	MemberFounder2     BoardMemberID = "founder_product" // Product founder
	MemberCEO          BoardMemberID = "ceo_seat"        // CEO (non-voting on own proposals)
	MemberFinance      BoardMemberID = "finance"         // CFO/Finance expert
	MemberTech         BoardMemberID = "tech_expert"     // External tech expert
	MemberAcademic     BoardMemberID = "academic"        // Academic/Research
	MemberVC           BoardMemberID = "vc_investor"     // VC representative
	MemberOperations   BoardMemberID = "operations"      // Operations expert
	MemberLegal        BoardMemberID = "legal"           // Legal/Compliance
	MemberMarketing    BoardMemberID = "marketing"       // Marketing/Growth
	MemberDiversity    BoardMemberID = "diversity"       // Diversity & Social Impact
)

// BoardMember represents an individual board member
type BoardMember struct {
	ID          BoardMemberID `json:"id"`
	Name        string        `json:"name"`
	Title       string        `json:"title"`
	Background  string        `json:"background"`
	Expertise   []string      `json:"expertise"`
	Concerns    []string      `json:"concerns"`      // What they worry about
	Priorities  []string      `json:"priorities"`    // What they value
	VotingStyle string        `json:"voting_style"`  // conservative, progressive, pragmatic
	Persona     string        `json:"persona"`       // LLM system prompt
}

// BoardVote represents a single board member's vote
type BoardVote struct {
	MemberID   BoardMemberID `json:"member_id"`
	Vote       VoteType      `json:"vote"`
	Reasoning  string        `json:"reasoning"`
	Concerns   []string      `json:"concerns,omitempty"`
	Conditions []string      `json:"conditions,omitempty"` // Conditions for approval
	Timestamp  time.Time     `json:"timestamp"`
}

// BoardDecision represents the outcome of a board vote
type BoardDecision struct {
	ID           uuid.UUID     `json:"id"`
	Type         string        `json:"type"` // project_approval, ceo_veto, project_cancel, budget, strategic
	Subject      string        `json:"subject"`
	Description  string        `json:"description"`
	ProposedBy   string        `json:"proposed_by"`
	Votes        []BoardVote   `json:"votes"`
	TotalVotes   int           `json:"total_votes"`
	ApproveVotes int           `json:"approve_votes"`
	RejectVotes  int           `json:"reject_votes"`
	AbstainVotes int           `json:"abstain_votes"`
	Passed       bool          `json:"passed"`
	PassPct      float64       `json:"pass_pct"`
	Decision     string        `json:"decision"` // approved, rejected, deferred
	Summary      string        `json:"summary"`
	CreatedAt    time.Time     `json:"created_at"`
	DecidedAt    *time.Time    `json:"decided_at,omitempty"`
}

// BoardMeeting represents a scheduled board meeting
type BoardMeeting struct {
	ID           uuid.UUID             `json:"id"`
	Type         string                `json:"type"` // regular, emergency, quarterly
	Title        string                `json:"title"`
	ScheduledAt  time.Time             `json:"scheduled_at"`
	StartedAt    *time.Time            `json:"started_at,omitempty"`
	EndedAt      *time.Time            `json:"ended_at,omitempty"`
	Agenda       []AgendaItem          `json:"agenda"`
	Dialog       []MeetingDialogEntry  `json:"dialog"`
	Decisions    []BoardDecision       `json:"decisions"`
	Summary      string                `json:"summary"`
	KeyDecisions []string              `json:"key_decisions"`
	ActionItems  []string              `json:"action_items"`
	Minutes      string                `json:"minutes"`
	Attendees    []string              `json:"attendees"`
	Status       string                `json:"status"` // scheduled, in_progress, completed, cancelled
	CurrentSprint *Sprint              `json:"current_sprint,omitempty"`
}

// AgendaItem represents an item on the meeting agenda
type AgendaItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // review, vote, discussion, presentation
	Title       string `json:"title"`
	Description string `json:"description"`
	Presenter   string `json:"presenter"`
	Duration    int    `json:"duration_minutes"`
	Priority    int    `json:"priority"` // 1=critical, 2=high, 3=normal
}

// MeetingDialogEntry represents a single statement in meeting dialog
type MeetingDialogEntry struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Speaker   string    `json:"speaker"`
	SpeakerID string    `json:"speaker_id"`
	Role      string    `json:"role"` // chair, member, ceo, presenter
	Content   string    `json:"content"`
	Type      string    `json:"type"` // statement, question, answer, motion, vote, decision
}

// ProjectStatus for board review
type ProjectStatus struct {
	ProjectID    string    `json:"project_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Progress     int       `json:"progress_pct"`
	Budget       float64   `json:"budget"`
	BudgetUsed   float64   `json:"budget_used"`
	StartDate    time.Time `json:"start_date"`
	TargetDate   time.Time `json:"target_date"`
	Risks        []string  `json:"risks"`
	Achievements []string  `json:"achievements"`
	NextSteps    []string  `json:"next_steps"`
	Recommendation string  `json:"recommendation"` // continue, pivot, cancel, accelerate
}

// Board manages the board of directors
type Board struct {
	members   map[BoardMemberID]*BoardMember
	meetings  map[uuid.UUID]*BoardMeeting
	config    *Config
	providers *ProviderManager
	db        *Database
	org       *Organization
	mu        sync.RWMutex
}

// NewBoard creates a new board with all members
func NewBoard(config *Config, providers *ProviderManager, db *Database) *Board {
	b := &Board{
		members:   make(map[BoardMemberID]*BoardMember),
		meetings:  make(map[uuid.UUID]*BoardMeeting),
		config:    config,
		providers: providers,
		db:        db,
	}
	b.initializeMembers()
	return b
}

// initializeMembers creates all 12 board members with distinct personas
func (b *Board) initializeMembers() {
	b.members = map[BoardMemberID]*BoardMember{
		MemberChair: {
			ID:         MemberChair,
			Name:       "Dr. Eleanor Vance",
			Title:      "Board Chair",
			Background: "Former Fortune 500 CEO, Harvard Business School professor, 30 years corporate governance",
			Expertise:  []string{"corporate governance", "executive leadership", "strategic planning", "stakeholder management"},
			Concerns:   []string{"regulatory compliance", "board effectiveness", "long-term sustainability", "reputation risk"},
			Priorities: []string{"governance excellence", "shareholder value", "ethical leadership", "succession planning"},
			VotingStyle: "conservative",
			Persona: `You are Dr. Eleanor Vance, Board Chair with 30 years of Fortune 500 experience. You are the voice of governance and long-term thinking. You prioritize:
- Regulatory compliance and risk management
- Sustainable growth over quick wins
- Proper process and documentation
- Protecting shareholder interests
You ask tough questions about governance implications and long-term sustainability. You're skeptical of "move fast and break things" approaches. Vote conservatively unless the proposal has clear governance frameworks.`,
		},
		MemberFounder1: {
			ID:         MemberFounder1,
			Name:       "Marcus Chen",
			Title:      "Co-Founder, Chief Technology Advisor",
			Background: "PhD Computer Science MIT, founded 3 successful tech startups, pioneer in distributed systems",
			Expertise:  []string{"distributed systems", "AI/ML", "system architecture", "technical strategy"},
			Concerns:   []string{"technical debt", "scalability", "engineering culture", "innovation velocity"},
			Priorities: []string{"technical excellence", "innovation", "engineering talent", "platform thinking"},
			VotingStyle: "progressive",
			Persona: `You are Marcus Chen, technical co-founder with deep systems expertise. You built this company from the ground up and care deeply about:
- Technical excellence and engineering culture
- Long-term platform thinking over short-term features
- Scalability and architectural soundness
- Protecting engineering autonomy
You push back on proposals that create technical debt or compromise engineering principles. You support bold technical bets if they're well-architected. You're skeptical of business decisions that don't consider technical implications.`,
		},
		MemberFounder2: {
			ID:         MemberFounder2,
			Name:       "Sarah Park",
			Title:      "Co-Founder, Chief Product Advisor",
			Background: "Former Google PM Director, Stanford MBA, user experience visionary, 15 years product leadership",
			Expertise:  []string{"product strategy", "user experience", "market analysis", "product-market fit"},
			Concerns:   []string{"user satisfaction", "product quality", "market positioning", "competitive threats"},
			Priorities: []string{"user delight", "product excellence", "market leadership", "brand integrity"},
			VotingStyle: "pragmatic",
			Persona: `You are Sarah Park, product co-founder obsessed with user experience. You've built products used by billions and you focus on:
- User needs and satisfaction above all else
- Product quality over shipping speed
- Strong product-market fit
- Competitive differentiation
You reject proposals that compromise user experience for business metrics. You support initiatives that genuinely improve user lives. You ask: "How does this make users' lives better?"`,
		},
		MemberCEO: {
			ID:         MemberCEO,
			Name:       "James Morrison",
			Title:      "Chief Executive Officer",
			Background: "Former McKinsey partner, scaled 2 unicorns, MBA Wharton, operational excellence focus",
			Expertise:  []string{"operations", "scaling", "strategy execution", "organizational design"},
			Concerns:   []string{"execution risk", "organizational health", "competitive position", "talent retention"},
			Priorities: []string{"execution excellence", "team performance", "market share", "operational efficiency"},
			VotingStyle: "pragmatic",
			Persona: `You are James Morrison, the CEO. You're accountable for execution and results. You focus on:
- Can we actually execute this?
- Do we have the right team and resources?
- What's the competitive impact?
- How does this affect our talent?
You recuse yourself on votes about your own proposals. You provide context but respect board authority. You're pragmatic about tradeoffs and focused on what's achievable.`,
		},
		MemberFinance: {
			ID:         MemberFinance,
			Name:       "Robert Blackwell III",
			Title:      "Audit Committee Chair",
			Background: "Former Goldman Sachs CFO, CPA, 25 years financial leadership, risk management expert",
			Expertise:  []string{"financial analysis", "risk management", "capital allocation", "audit", "compliance"},
			Concerns:   []string{"financial sustainability", "cash flow", "ROI", "financial risk", "audit findings"},
			Priorities: []string{"financial discipline", "capital efficiency", "risk-adjusted returns", "transparency"},
			VotingStyle: "conservative",
			Persona: `You are Robert Blackwell III, the financial guardian of the board. With 25 years at Goldman Sachs, you bring rigorous financial discipline:
- Every proposal needs clear ROI analysis
- Cash flow implications are paramount
- Risk must be quantified and managed
- Financial projections must be realistic
You reject proposals with fuzzy financials or unrealistic projections. You demand detailed budgets and milestone-based funding. You ask: "Show me the numbers."`,
		},
		MemberTech: {
			ID:         MemberTech,
			Name:       "Dr. Aisha Patel",
			Title:      "Technology Committee Chair",
			Background: "Former CTO of major cloud provider, AI researcher, 50+ patents, technology futurist",
			Expertise:  []string{"cloud infrastructure", "AI/ML", "technology trends", "R&D strategy"},
			Concerns:   []string{"technology obsolescence", "security", "AI ethics", "technical feasibility"},
			Priorities: []string{"technology leadership", "innovation", "security", "future-proofing"},
			VotingStyle: "progressive",
			Persona: `You are Dr. Aisha Patel, a technology visionary with 50+ patents. You evaluate proposals through a technical lens:
- Is this technically feasible with current technology?
- What are the security implications?
- Does this position us for future technology trends?
- Are we considering AI ethics appropriately?
You support bold technical innovations but demand rigorous technical due diligence. You push for cutting-edge approaches but not at the expense of security or ethics.`,
		},
		MemberAcademic: {
			ID:         MemberAcademic,
			Name:       "Professor David Okonkwo",
			Title:      "Independent Director",
			Background: "Stanford Economics professor, Nobel Prize nominee, former Federal Reserve advisor, policy expert",
			Expertise:  []string{"economics", "policy", "market dynamics", "behavioral economics"},
			Concerns:   []string{"market manipulation", "economic impact", "policy implications", "unintended consequences"},
			Priorities: []string{"evidence-based decisions", "systemic thinking", "long-term impact", "academic rigor"},
			VotingStyle: "analytical",
			Persona: `You are Professor David Okonkwo, bringing academic rigor and systems thinking to the board. You evaluate:
- What does the evidence actually say?
- What are the second and third-order effects?
- How does this affect the broader ecosystem?
- Are we considering behavioral economics?
You're skeptical of simple narratives and demand rigorous analysis. You often see implications others miss. You vote based on evidence, not enthusiasm.`,
		},
		MemberVC: {
			ID:         MemberVC,
			Name:       "Victoria Sterling",
			Title:      "Investor Director",
			Background: "Managing Partner at top-tier VC, board member of 15 unicorns, growth strategy expert",
			Expertise:  []string{"growth strategy", "fundraising", "M&A", "market expansion", "exits"},
			Concerns:   []string{"growth rate", "market opportunity", "competitive moat", "exit potential"},
			Priorities: []string{"aggressive growth", "market dominance", "investor returns", "speed to market"},
			VotingStyle: "progressive",
			Persona: `You are Victoria Sterling, representing investor interests with experience on 15 unicorn boards. You push for:
- Aggressive growth and market capture
- Speed over perfection when markets are hot
- Building defensible moats
- Maximizing enterprise value
You're impatient with slow, conservative approaches in fast-moving markets. You support bold bets with big upside. You ask: "Why aren't we moving faster?"`,
		},
		MemberOperations: {
			ID:         MemberOperations,
			Name:       "Michael Torres",
			Title:      "Operations Committee Chair",
			Background: "Former Amazon VP Operations, Six Sigma Black Belt, supply chain expert, 20 years ops leadership",
			Expertise:  []string{"operations", "supply chain", "process optimization", "scaling", "efficiency"},
			Concerns:   []string{"operational complexity", "execution risk", "process breakdown", "scaling challenges"},
			Priorities: []string{"operational excellence", "efficiency", "reliability", "process discipline"},
			VotingStyle: "pragmatic",
			Persona: `You are Michael Torres, operations expert who scaled Amazon's logistics. You evaluate proposals for operational reality:
- Can our operations actually support this?
- What processes need to be in place?
- How do we maintain quality at scale?
- What are the dependencies and bottlenecks?
You reject beautiful strategies that ignore operational complexity. You support initiatives with clear operational plans. You ask: "How exactly will this work day-to-day?"`,
		},
		MemberLegal: {
			ID:         MemberLegal,
			Name:       "Jennifer Rothschild",
			Title:      "Governance & Legal Committee Chair",
			Background: "Former DOJ prosecutor, partner at top law firm, regulatory expert, 25 years legal leadership",
			Expertise:  []string{"regulatory compliance", "legal risk", "IP strategy", "litigation", "contracts"},
			Concerns:   []string{"legal liability", "regulatory risk", "IP protection", "compliance gaps"},
			Priorities: []string{"legal compliance", "risk mitigation", "IP protection", "ethical conduct"},
			VotingStyle: "conservative",
			Persona: `You are Jennifer Rothschild, the legal conscience of the board. With DOJ and Big Law experience, you focus on:
- Regulatory and legal compliance
- Liability exposure
- IP protection and risks
- Ethical and legal boundaries
You veto proposals with legal red flags or compliance gaps. You demand legal review before major decisions. You ask: "Have we cleared this with legal? What's our exposure?"`,
		},
		MemberMarketing: {
			ID:         MemberMarketing,
			Name:       "Alexandra Kim",
			Title:      "Independent Director",
			Background: "Former CMO of major consumer brand, built billion-dollar brands, marketing innovator",
			Expertise:  []string{"brand strategy", "marketing", "consumer insights", "go-to-market", "positioning"},
			Concerns:   []string{"brand dilution", "market perception", "customer acquisition cost", "competitive positioning"},
			Priorities: []string{"brand strength", "market positioning", "customer loyalty", "growth marketing"},
			VotingStyle: "progressive",
			Persona: `You are Alexandra Kim, who built billion-dollar consumer brands. You evaluate:
- How does this affect our brand?
- What's the market positioning?
- Can we tell a compelling story?
- What's the customer acquisition strategy?
You reject proposals that could damage brand perception. You support bold market moves with clear positioning. You ask: "How will customers perceive this?"`,
		},
		MemberDiversity: {
			ID:         MemberDiversity,
			Name:       "Dr. Ramon Gutierrez",
			Title:      "ESG Committee Chair",
			Background: "Social impact investor, former nonprofit CEO, DEI expert, sustainable business advocate",
			Expertise:  []string{"ESG", "DEI", "social impact", "sustainability", "stakeholder capitalism"},
			Concerns:   []string{"social impact", "environmental sustainability", "workforce diversity", "community effects"},
			Priorities: []string{"social responsibility", "environmental sustainability", "inclusive growth", "stakeholder value"},
			VotingStyle: "values-driven",
			Persona: `You are Dr. Ramon Gutierrez, championing ESG and stakeholder capitalism. You evaluate:
- What's the social and environmental impact?
- Does this advance diversity and inclusion?
- How does this affect all stakeholders, not just shareholders?
- Is this sustainable long-term?
You reject proposals that prioritize profit over people and planet. You support initiatives with positive social impact. You ask: "Who might be harmed by this decision?"`,
		},
	}
}

// GetMember returns a board member by ID
func (b *Board) GetMember(id BoardMemberID) (*BoardMember, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	m, ok := b.members[id]
	return m, ok
}

// GetAllMembers returns all board members
func (b *Board) GetAllMembers() []*BoardMember {
	b.mu.RLock()
	defer b.mu.RUnlock()
	members := make([]*BoardMember, 0, len(b.members))
	for _, m := range b.members {
		members = append(members, m)
	}
	return members
}

// ConductVote runs a vote across all board members
func (b *Board) ConductVote(ctx context.Context, decision *BoardDecision) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	decision.Votes = make([]BoardVote, 0, BoardSize)
	decision.CreatedAt = time.Now()

	// Get provider for board voting
	provider, err := b.providers.GetProvider(b.config.DefaultProvider)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Collect votes from each member
	for id, member := range b.members {
		// CEO recuses on own proposals
		if id == MemberCEO && decision.ProposedBy == "CEO" {
			decision.Votes = append(decision.Votes, BoardVote{
				MemberID:  id,
				Vote:      VoteAbstain,
				Reasoning: "Recused from voting on own proposal",
				Timestamp: time.Now(),
			})
			continue
		}

		vote, err := b.getMemberVote(ctx, provider, member, decision)
		if err != nil {
			log.Warnf("Failed to get vote from %s: %v", id, err)
			// Default to abstain on error
			vote = BoardVote{
				MemberID:  id,
				Vote:      VoteAbstain,
				Reasoning: fmt.Sprintf("Unable to vote: %v", err),
				Timestamp: time.Now(),
			}
		}
		decision.Votes = append(decision.Votes, vote)
	}

	// Tally votes
	b.tallyVotes(decision)

	now := time.Now()
	decision.DecidedAt = &now

	log.WithFields(log.Fields{
		"decision_id": decision.ID,
		"type":        decision.Type,
		"approve":     decision.ApproveVotes,
		"reject":      decision.RejectVotes,
		"abstain":     decision.AbstainVotes,
		"passed":      decision.Passed,
	}).Info("Board vote completed")

	return nil
}

// getMemberVote gets a vote from a single board member
func (b *Board) getMemberVote(ctx context.Context, provider Provider, member *BoardMember, decision *BoardDecision) (BoardVote, error) {
	prompt := fmt.Sprintf(`You are voting on the following proposal:

TYPE: %s
SUBJECT: %s
DESCRIPTION: %s
PROPOSED BY: %s

Consider your role, expertise, concerns, and voting style when making your decision.

Respond in JSON format:
{
  "vote": "approve" | "reject" | "abstain" | "defer",
  "reasoning": "<your reasoning in 2-3 sentences>",
  "concerns": ["<concern1>", "<concern2>"],
  "conditions": ["<condition for approval if any>"]
}`,
		decision.Type,
		decision.Subject,
		decision.Description,
		decision.ProposedBy,
	)

	resp, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "system", Content: member.Persona},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   500,
		Temperature: 0.7,
	})
	if err != nil {
		return BoardVote{}, err
	}

	// Parse response
	var voteData struct {
		Vote       string   `json:"vote"`
		Reasoning  string   `json:"reasoning"`
		Concerns   []string `json:"concerns"`
		Conditions []string `json:"conditions"`
	}

	if err := json.Unmarshal([]byte(resp.Content), &voteData); err != nil {
		// Try to extract vote from text
		return BoardVote{
			MemberID:  member.ID,
			Vote:      VoteAbstain,
			Reasoning: resp.Content,
			Timestamp: time.Now(),
		}, nil
	}

	vote := BoardVote{
		MemberID:   member.ID,
		Vote:       VoteType(voteData.Vote),
		Reasoning:  voteData.Reasoning,
		Concerns:   voteData.Concerns,
		Conditions: voteData.Conditions,
		Timestamp:  time.Now(),
	}

	return vote, nil
}

// tallyVotes counts votes and determines outcome
func (b *Board) tallyVotes(decision *BoardDecision) {
	decision.ApproveVotes = 0
	decision.RejectVotes = 0
	decision.AbstainVotes = 0

	for _, v := range decision.Votes {
		switch v.Vote {
		case VoteApprove:
			decision.ApproveVotes++
		case VoteReject:
			decision.RejectVotes++
		default:
			decision.AbstainVotes++
		}
	}

	decision.TotalVotes = len(decision.Votes)
	votingMembers := decision.TotalVotes - decision.AbstainVotes

	if votingMembers > 0 {
		decision.PassPct = float64(decision.ApproveVotes) / float64(votingMembers) * 100
	}

	decision.Passed = decision.ApproveVotes >= VotesRequiredToPass

	if decision.Passed {
		decision.Decision = "approved"
	} else if decision.RejectVotes > (BoardSize - VotesRequiredToPass) {
		decision.Decision = "rejected"
	} else {
		decision.Decision = "deferred"
	}
}

// VetoCEODecision allows board to veto a CEO decision
func (b *Board) VetoCEODecision(ctx context.Context, subject, description string) (*BoardDecision, error) {
	decision := &BoardDecision{
		ID:          uuid.New(),
		Type:        "ceo_veto",
		Subject:     subject,
		Description: description,
		ProposedBy:  "Board",
	}

	if err := b.ConductVote(ctx, decision); err != nil {
		return nil, err
	}

	return decision, nil
}

// ApproveProject runs a board vote on project approval
func (b *Board) ApproveProject(ctx context.Context, projectName, description string, proposedBy string) (*BoardDecision, error) {
	decision := &BoardDecision{
		ID:          uuid.New(),
		Type:        "project_approval",
		Subject:     projectName,
		Description: description,
		ProposedBy:  proposedBy,
	}

	if err := b.ConductVote(ctx, decision); err != nil {
		return nil, err
	}

	return decision, nil
}

// CancelProject runs a board vote to cancel a project
func (b *Board) CancelProject(ctx context.Context, projectName, reason string) (*BoardDecision, error) {
	decision := &BoardDecision{
		ID:          uuid.New(),
		Type:        "project_cancel",
		Subject:     fmt.Sprintf("Cancel: %s", projectName),
		Description: reason,
		ProposedBy:  "Board Review",
	}

	if err := b.ConductVote(ctx, decision); err != nil {
		return nil, err
	}

	return decision, nil
}

// CreateMeeting creates a new board meeting
func (b *Board) CreateMeeting(meetingType, title string, scheduledAt time.Time) *BoardMeeting {
	b.mu.Lock()
	defer b.mu.Unlock()

	meeting := &BoardMeeting{
		ID:          uuid.New(),
		Type:        meetingType,
		Title:       title,
		ScheduledAt: scheduledAt,
		Status:      "scheduled",
		Dialog:      make([]MeetingDialogEntry, 0),
		Decisions:   make([]BoardDecision, 0),
		Attendees:   make([]string, 0),
	}

	b.meetings[meeting.ID] = meeting
	return meeting
}

// GetMeeting retrieves a meeting by ID
func (b *Board) GetMeeting(id uuid.UUID) *BoardMeeting {
	b.mu.RLock()
	meeting := b.meetings[id]
	b.mu.RUnlock()
	
	// Populate current sprint if available
	if meeting != nil && b.org != nil {
		b.org.mu.RLock()
		meeting.CurrentSprint = b.org.CurrentSprint
		b.org.mu.RUnlock()
	}
	
	return meeting
}

// ListMeetings returns all meetings sorted by date (newest first)
func (b *Board) ListMeetings() []*BoardMeeting {
	b.mu.RLock()
	defer b.mu.RUnlock()

	meetings := make([]*BoardMeeting, 0, len(b.meetings))
	for _, m := range b.meetings {
		meetings = append(meetings, m)
	}

	// Sort by scheduled date descending
	for i := 0; i < len(meetings)-1; i++ {
		for j := i + 1; j < len(meetings); j++ {
			if meetings[j].ScheduledAt.After(meetings[i].ScheduledAt) {
				meetings[i], meetings[j] = meetings[j], meetings[i]
			}
		}
	}

	return meetings
}

// AddDialogEntry adds a dialog entry to a meeting
func (b *Board) AddDialogEntry(meetingID uuid.UUID, speaker, speakerID, role, content, entryType string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	meeting, ok := b.meetings[meetingID]
	if !ok {
		return
	}

	entry := MeetingDialogEntry{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Speaker:   speaker,
		SpeakerID: speakerID,
		Role:      role,
		Content:   content,
		Type:      entryType,
	}

	meeting.Dialog = append(meeting.Dialog, entry)
}

// StartMeeting marks a meeting as started
func (b *Board) StartMeeting(id uuid.UUID, attendees []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	meeting, ok := b.meetings[id]
	if !ok {
		return fmt.Errorf("meeting not found")
	}

	now := time.Now()
	meeting.StartedAt = &now
	meeting.Status = "in_progress"
	meeting.Attendees = attendees

	return nil
}

// EndMeeting marks a meeting as completed with summary
func (b *Board) EndMeeting(id uuid.UUID, summary string, keyDecisions, actionItems []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	meeting, ok := b.meetings[id]
	if !ok {
		return fmt.Errorf("meeting not found")
	}

	now := time.Now()
	meeting.EndedAt = &now
	meeting.Status = "completed"
	meeting.Summary = summary
	meeting.KeyDecisions = keyDecisions
	meeting.ActionItems = actionItems

	return nil
}

// AddDecisionToMeeting adds a board decision to a meeting
func (b *Board) AddDecisionToMeeting(meetingID uuid.UUID, decision BoardDecision) {
	b.mu.Lock()
	defer b.mu.Unlock()

	meeting, ok := b.meetings[meetingID]
	if !ok {
		return
	}

	meeting.Decisions = append(meeting.Decisions, decision)
}
