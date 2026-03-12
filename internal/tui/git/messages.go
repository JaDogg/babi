package git

// Navigation messages
type navigateToFilesMsg struct{ reload bool }
type navigateToCommitMsg struct{ files []string }
type navigateToResultMsg struct {
	output string
	err    error
}
type commitDoneMsg struct{}
type diffLoadedMsg struct {
	path    string
	content string
}
