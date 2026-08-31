package frontend

// drain empties pending notifications so a burst redraws once.
func drain(events <-chan struct{}) {
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		default:
			return
		}
	}
}
