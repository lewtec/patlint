package project

// EmbeddedSource is one re-parsed region inside a host file (e.g. Svelte <script>).
type EmbeddedSource struct {
	Offset   uint32
	Source   []byte
	Language string
	FileHint string
}
