package domain

type Image struct {
	ID            string
	ArtworkID     string
	ClientImageID string
	StorageKey    string
	SourceSHA256  string
	ContentType   string
	Position      int
	Width         int
	Height        int
	ByteSize      int
	Blurhash      string
}
