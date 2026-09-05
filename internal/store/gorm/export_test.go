package gorm

import "gorm.io/gorm"

// UnsafeDB exposes the GORM handle for tests that need to drive the
// database directly. Production code must use the store interfaces.
func (s *Store) UnsafeDB() *gorm.DB { return s.db }
