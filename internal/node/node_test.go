package node

import (
	"testing"

	"github.com/jasondostal/tresbbs/domain"
)

// TestRegisterPopulatesWhosOnline is the regression test for the stub-manager
// bug: RegisterNode used to be a no-op, so Who's-Online showed blank users.
func TestRegisterPopulatesWhosOnline(t *testing.T) {
	m := NewManager(4)
	n := m.AllocNode()
	if n != 1 {
		t.Fatalf("first AllocNode = %d, want 1", n)
	}
	m.RegisterNode(&domain.NodeStatus{
		NodeNumber: n, UserName: "Claude", UserAlias: "ClaudeTron",
		SecurityLevel: 50, BaudRate: "Telnet", Activity: "Main Menu",
	})
	nodes := m.GetNodes()
	if len(nodes) != 1 {
		t.Fatalf("Who's Online = %d nodes, want 1", len(nodes))
	}
	if nodes[0].UserAlias != "ClaudeTron" {
		t.Errorf("alias = %q, want ClaudeTron (was blank with the stub)", nodes[0].UserAlias)
	}
	if nodes[0].Activity != "Main Menu" {
		t.Errorf("activity = %q, want Main Menu", nodes[0].Activity)
	}
}

func TestDuplicateLoginDetected(t *testing.T) {
	m := NewManager(4)
	n := m.AllocNode()
	m.RegisterNode(&domain.NodeStatus{NodeNumber: n, UserName: "Claude", UserAlias: "ClaudeTron"})
	if dup, onNode, _ := m.IsDuplicateLogin("Claude"); !dup || onNode != 1 {
		t.Errorf("IsDuplicateLogin(Claude) = %v,%d; want true,1 (stub always returned false)", dup, onNode)
	}
	if dup, _, _ := m.IsDuplicateLogin("Nobody"); dup {
		t.Error("unknown user flagged as duplicate login")
	}
}

func TestPagingDelivers(t *testing.T) {
	m := NewManager(4)
	a, b := m.AllocNode(), m.AllocNode()
	m.RegisterNode(&domain.NodeStatus{NodeNumber: a, UserAlias: "A"})
	m.RegisterNode(&domain.NodeStatus{NodeNumber: b, UserAlias: "B"})
	if err := m.SendPage(a, b, "A"); err != nil {
		t.Fatalf("SendPage: %v", err)
	}
	if pending, from, _ := m.CheckPage(b); !pending || from != "A" {
		t.Errorf("CheckPage(b) = %v,%q; want true,\"A\" (stub never delivered)", pending, from)
	}
}
