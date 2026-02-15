package main

// SetOrganization sets the organization reference for sprint context
func (b *Board) SetOrganization(org *Organization) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.org = org
}
