package state

func (s *State) Close() {
	s.cancel()
}
