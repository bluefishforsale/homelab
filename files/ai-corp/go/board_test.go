package main

import (
	"testing"
)

func TestBoardConstants(t *testing.T) {
	if BoardSize != 12 {
		t.Errorf("Expected BoardSize 12, got %d", BoardSize)
	}

	if VoteMajorityPct != 70 {
		t.Errorf("Expected VoteMajorityPct 70, got %d", VoteMajorityPct)
	}

	if VotesRequiredToPass != 9 {
		t.Errorf("Expected VotesRequiredToPass 9, got %d", VotesRequiredToPass)
	}
}

func TestBoardMemberIDs(t *testing.T) {
	expectedIDs := []BoardMemberID{
		MemberChair,
		MemberFounder1,
		MemberFounder2,
		MemberCEO,
		MemberFinance,
		MemberTech,
		MemberAcademic,
		MemberVC,
		MemberOperations,
		MemberLegal,
		MemberMarketing,
		MemberDiversity,
	}

	if len(expectedIDs) != BoardSize {
		t.Errorf("Expected %d member IDs, got %d", BoardSize, len(expectedIDs))
	}
}

func TestVoteTypes(t *testing.T) {
	tests := []struct {
		voteType VoteType
		expected string
	}{
		{VoteApprove, "approve"},
		{VoteReject, "reject"},
		{VoteAbstain, "abstain"},
		{VoteDefer, "defer"},
	}

	for _, tt := range tests {
		if string(tt.voteType) != tt.expected {
			t.Errorf("Expected vote type %s, got %s", tt.expected, tt.voteType)
		}
	}
}

func TestBoardCreation(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	
	board := NewBoard(cfg, pm, nil)
	
	members := board.GetAllMembers()
	if len(members) != BoardSize {
		t.Errorf("Expected %d board members, got %d", BoardSize, len(members))
	}
}

func TestGetBoardMember(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	board := NewBoard(cfg, pm, nil)

	// Test getting existing member
	chair, ok := board.GetMember(MemberChair)
	if !ok {
		t.Fatal("Expected to find board chair")
	}

	if chair.Name == "" {
		t.Error("Expected chair to have a name")
	}
	if chair.Title != "Board Chair" {
		t.Errorf("Expected chair title 'Board Chair', got %s", chair.Title)
	}
	if chair.Persona == "" {
		t.Error("Expected chair to have a persona")
	}

	// Test getting non-existent member
	_, ok = board.GetMember("nonexistent")
	if ok {
		t.Error("Expected not to find nonexistent member")
	}
}

func TestBoardMemberAttributes(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	board := NewBoard(cfg, pm, nil)

	members := board.GetAllMembers()

	for _, m := range members {
		// Each member should have required attributes
		if m.ID == "" {
			t.Error("Member missing ID")
		}
		if m.Name == "" {
			t.Errorf("Member %s missing name", m.ID)
		}
		if m.Title == "" {
			t.Errorf("Member %s missing title", m.ID)
		}
		if m.Background == "" {
			t.Errorf("Member %s missing background", m.ID)
		}
		if len(m.Expertise) == 0 {
			t.Errorf("Member %s missing expertise", m.ID)
		}
		if len(m.Concerns) == 0 {
			t.Errorf("Member %s missing concerns", m.ID)
		}
		if len(m.Priorities) == 0 {
			t.Errorf("Member %s missing priorities", m.ID)
		}
		if m.VotingStyle == "" {
			t.Errorf("Member %s missing voting style", m.ID)
		}
		if m.Persona == "" {
			t.Errorf("Member %s missing persona", m.ID)
		}
	}
}

func TestBoardMemberDiversity(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	board := NewBoard(cfg, pm, nil)

	members := board.GetAllMembers()

	// Check for diverse voting styles
	votingStyles := make(map[string]int)
	for _, m := range members {
		votingStyles[m.VotingStyle]++
	}

	// Should have multiple voting styles
	if len(votingStyles) < 3 {
		t.Errorf("Expected at least 3 different voting styles, got %d", len(votingStyles))
	}
}

func TestTallyVotes(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	board := NewBoard(cfg, pm, nil)

	decision := &BoardDecision{
		Votes: []BoardVote{
			{MemberID: MemberChair, Vote: VoteApprove},
			{MemberID: MemberFounder1, Vote: VoteApprove},
			{MemberID: MemberFounder2, Vote: VoteApprove},
			{MemberID: MemberCEO, Vote: VoteAbstain},
			{MemberID: MemberFinance, Vote: VoteApprove},
			{MemberID: MemberTech, Vote: VoteApprove},
			{MemberID: MemberAcademic, Vote: VoteApprove},
			{MemberID: MemberVC, Vote: VoteApprove},
			{MemberID: MemberOperations, Vote: VoteApprove},
			{MemberID: MemberLegal, Vote: VoteReject},
			{MemberID: MemberMarketing, Vote: VoteApprove},
			{MemberID: MemberDiversity, Vote: VoteApprove},
		},
	}

	board.tallyVotes(decision)

	if decision.ApproveVotes != 10 {
		t.Errorf("Expected 10 approve votes, got %d", decision.ApproveVotes)
	}
	if decision.RejectVotes != 1 {
		t.Errorf("Expected 1 reject vote, got %d", decision.RejectVotes)
	}
	if decision.AbstainVotes != 1 {
		t.Errorf("Expected 1 abstain vote, got %d", decision.AbstainVotes)
	}
	if !decision.Passed {
		t.Error("Expected decision to pass with 10 approve votes")
	}
	if decision.Decision != "approved" {
		t.Errorf("Expected decision 'approved', got %s", decision.Decision)
	}
}

func TestTallyVotesRejection(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	board := NewBoard(cfg, pm, nil)

	decision := &BoardDecision{
		Votes: []BoardVote{
			{MemberID: MemberChair, Vote: VoteReject},
			{MemberID: MemberFounder1, Vote: VoteReject},
			{MemberID: MemberFounder2, Vote: VoteReject},
			{MemberID: MemberCEO, Vote: VoteReject},
			{MemberID: MemberFinance, Vote: VoteApprove},
			{MemberID: MemberTech, Vote: VoteApprove},
			{MemberID: MemberAcademic, Vote: VoteApprove},
			{MemberID: MemberVC, Vote: VoteApprove},
			{MemberID: MemberOperations, Vote: VoteApprove},
			{MemberID: MemberLegal, Vote: VoteApprove},
			{MemberID: MemberMarketing, Vote: VoteApprove},
			{MemberID: MemberDiversity, Vote: VoteApprove},
		},
	}

	board.tallyVotes(decision)

	if decision.ApproveVotes != 8 {
		t.Errorf("Expected 8 approve votes, got %d", decision.ApproveVotes)
	}
	if decision.RejectVotes != 4 {
		t.Errorf("Expected 4 reject votes, got %d", decision.RejectVotes)
	}
	if decision.Passed {
		t.Error("Expected decision to fail with only 8 approve votes")
	}
}

func TestTallyVotesBorderline(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)
	board := NewBoard(cfg, pm, nil)

	// Exactly 9 approve votes (70% threshold)
	decision := &BoardDecision{
		Votes: []BoardVote{
			{MemberID: MemberChair, Vote: VoteApprove},
			{MemberID: MemberFounder1, Vote: VoteApprove},
			{MemberID: MemberFounder2, Vote: VoteApprove},
			{MemberID: MemberCEO, Vote: VoteApprove},
			{MemberID: MemberFinance, Vote: VoteApprove},
			{MemberID: MemberTech, Vote: VoteApprove},
			{MemberID: MemberAcademic, Vote: VoteApprove},
			{MemberID: MemberVC, Vote: VoteApprove},
			{MemberID: MemberOperations, Vote: VoteApprove},
			{MemberID: MemberLegal, Vote: VoteReject},
			{MemberID: MemberMarketing, Vote: VoteReject},
			{MemberID: MemberDiversity, Vote: VoteReject},
		},
	}

	board.tallyVotes(decision)

	if !decision.Passed {
		t.Error("Expected decision to pass with exactly 9 approve votes")
	}
}
