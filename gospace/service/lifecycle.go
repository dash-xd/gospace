package service

// activateThenEvict establishes the active-router pin before enforcing the
// bounded WASM cache. This prevents a newly loaded router from being selected
// as the only evictable entry while an older active router is still pinned.
func (s *Service) activateThenEvict(name string) error {
	if err := s.worker.Activate(name); err != nil {
		s.evictWASMIfNeeded()
		return err
	}
	s.evictWASMIfNeeded()
	return nil
}
