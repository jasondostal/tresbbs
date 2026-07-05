// Package node implements multinode coordination for tresbbs.
//
// The original TriBBS used file-based semaphores on a shared network drive
// for multinode coordination. Each node wrote its status to %s\NODE%d.%d
// files, and other nodes read these files to see who was online.
//
// For tresbbs, we use in-memory coordination with mutex-protected state.
// This is faster and more reliable than file-based coordination, while
// maintaining the same logical behavior.
//
// The node manager tracks:
//   - Which nodes are active
//   - Who is logged in on each node
//   - Chat requests between nodes
//   - Inter-node messaging
package node

import (
	"fmt"
	"sync"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// Manager coordinates multiple BBS nodes.
type Manager struct {
	mu       sync.RWMutex
	nodes    map[int]*NodeState
	maxNodes int
}

// NodeState represents the state of a single node.
type NodeState struct {
	Number      int
	Active      bool
	UserName    string
	UserAlias   string
	SecurityLevel int
	BaudRate    string
	Activity    string
	LoginTime   time.Time
	ChatAvail   bool
	PagePending bool
	PageFrom    string
	PageTo      int
	ChatTarget  int
	ChatActive  bool
	ChatPartner string
}

// NewManager creates a new node manager.
func NewManager(maxNodes int) *Manager {
	return &Manager{
		nodes:    make(map[int]*NodeState),
		maxNodes: maxNodes,
	}
}

// AllocNode allocates the next available node number.
func (m *Manager) AllocNode() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := 1; i <= m.maxNodes; i++ {
		if _, exists := m.nodes[i]; !exists {
			m.nodes[i] = &NodeState{
				Number: i,
			}
			return i
		}
	}
	return 0 // no nodes available
}

// FreeNode releases a node number.
func (m *Manager) FreeNode(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, n)
}

// ActivateNode marks a node as active with user information.
func (m *Manager) ActivateNode(n int, user *domain.User, baudRate string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.nodes[n]; ok {
		node.Active = true
		node.UserName = user.Name
		node.UserAlias = user.Alias
		node.SecurityLevel = user.SecurityLevel
		node.BaudRate = baudRate
		node.Activity = "Main Menu"
		node.LoginTime = time.Now()
		node.ChatAvail = true
	}
}

// DeactivateNode marks a node as inactive.
func (m *Manager) DeactivateNode(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.nodes[n]; ok {
		node.Active = false
		node.UserName = ""
		node.UserAlias = ""
		node.Activity = ""
		node.ChatActive = false
		node.ChatPartner = ""
	}
}

// UpdateActivity updates what a node is doing.
func (m *Manager) UpdateActivity(n int, activity string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.nodes[n]; ok {
		node.Activity = activity
	}
}

// GetNodes returns the status of all nodes.
func (m *Manager) GetNodes() []domain.NodeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []domain.NodeStatus
	for _, node := range m.nodes {
		if node.Active {
			result = append(result, domain.NodeStatus{
				NodeNumber:    node.Number,
				Active:        true,
				UserName:      node.UserName,
				UserAlias:     node.UserAlias,
				SecurityLevel: node.SecurityLevel,
				BaudRate:      node.BaudRate,
				Activity:      node.Activity,
				LoginTime:     node.LoginTime,
				ChatAvail:     node.ChatAvail,
				PagePending:   node.PagePending,
				PageFrom:      node.PageFrom,
			})
		}
	}
	return result
}

// GetNode returns the status of a specific node.
func (m *Manager) GetNode(n int) (*domain.NodeStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.nodes[n]
	if !ok {
		return nil, fmt.Errorf("node %d not found", n)
	}

	return &domain.NodeStatus{
		NodeNumber:    node.Number,
		Active:        node.Active,
		UserName:      node.UserName,
		UserAlias:     node.UserAlias,
		SecurityLevel: node.SecurityLevel,
		BaudRate:      node.BaudRate,
		Activity:      node.Activity,
		LoginTime:     node.LoginTime,
		ChatAvail:     node.ChatAvail,
		PagePending:   node.PagePending,
		PageFrom:      node.PageFrom,
	}, nil
}

// SendPage sends a chat page from one node to another.
func (m *Manager) SendPage(fromNode, toNode int, fromAlias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	target, ok := m.nodes[toNode]
	if !ok {
		return fmt.Errorf("node %d not found", toNode)
	}
	if !target.Active {
		return fmt.Errorf("node %d is not active", toNode)
	}
	if !target.ChatAvail {
		return fmt.Errorf("node %d has paging disabled", toNode)
	}

	target.PagePending = true
	target.PageFrom = fromAlias

	// Track who paged whom
	if source, ok := m.nodes[fromNode]; ok {
		source.PageTo = toNode
	}

	return nil
}

// CheckPage checks if a node has a pending page.
func (m *Manager) CheckPage(nodeNum int) (bool, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.nodes[nodeNum]
	if !ok {
		return false, "", fmt.Errorf("node %d not found", nodeNum)
	}

	return node.PagePending, node.PageFrom, nil
}

// ClearPage clears a pending page.
func (m *Manager) ClearPage(nodeNum int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.nodes[nodeNum]; ok {
		node.PagePending = false
		node.PageFrom = ""
	}
	return nil
}

// StartChat initiates a chat session between two nodes.
func (m *Manager) StartChat(node1, node2 int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	n1, ok1 := m.nodes[node1]
	n2, ok2 := m.nodes[node2]
	if !ok1 || !ok2 {
		return fmt.Errorf("one or both nodes not found")
	}
	if !n1.Active || !n2.Active {
		return fmt.Errorf("one or both nodes not active")
	}

	n1.ChatActive = true
	n1.ChatTarget = node2
	n1.ChatPartner = n2.UserAlias
	n1.Activity = fmt.Sprintf("Chat with %s", n2.UserAlias)

	n2.ChatActive = true
	n2.ChatTarget = node1
	n2.ChatPartner = n1.UserAlias
	n2.Activity = fmt.Sprintf("Chat with %s", n1.UserAlias)

	return nil
}

// EndChat ends a chat session for a node.
func (m *Manager) EndChat(nodeNum int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	node, ok := m.nodes[nodeNum]
	if !ok {
		return
	}

	// End chat on both sides
	if node.ChatActive {
		if target, ok := m.nodes[node.ChatTarget]; ok {
			target.ChatActive = false
			target.ChatTarget = 0
			target.ChatPartner = ""
			target.Activity = "Main Menu"
		}
	}

	node.ChatActive = false
	node.ChatTarget = 0
	node.ChatPartner = ""
	node.Activity = "Main Menu"
}

// IsDuplicateLogin checks if a user is already logged in on another node.
func (m *Manager) IsDuplicateLogin(userName string) (bool, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, node := range m.nodes {
		if node.Active && node.UserName == userName {
			return true, node.Number, nil
		}
	}
	return false, 0, nil
}

// RegisterNode implements port.NodePort (no-op for in-memory manager).
func (m *Manager) RegisterNode(status *domain.NodeStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[status.NodeNumber]
	if !ok {
		node = &NodeState{Number: status.NodeNumber}
		m.nodes[status.NodeNumber] = node
	}
	node.Active = true
	node.UserName = status.UserName
	node.UserAlias = status.UserAlias
	node.SecurityLevel = status.SecurityLevel
	node.BaudRate = status.BaudRate
	if status.Activity != "" {
		node.Activity = status.Activity
	} else {
		node.Activity = "Main Menu"
	}
	node.LoginTime = time.Now()
	node.ChatAvail = true
	return nil
}

// UpdateNode implements port.NodePort.
func (m *Manager) UpdateNode(status *domain.NodeStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.nodes[status.NodeNumber]; ok {
		node.Activity = status.Activity
		// NOTE: ChatAvail is deliberately NOT updated here — the activity
		// update runs every main-loop iteration and would otherwise wipe the
		// node's page-availability. Use SetChatAvail to change it.
	}
	return nil
}

// SetChatAvail toggles whether a node accepts inter-node chat pages.
func (m *Manager) SetChatAvail(nodeNumber int, avail bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.nodes[nodeNumber]; ok {
		node.ChatAvail = avail
	}
	return nil
}

// UnregisterNode implements port.NodePort.
func (m *Manager) UnregisterNode(nodeNumber int) error {
	m.FreeNode(nodeNumber)
	return nil
}
