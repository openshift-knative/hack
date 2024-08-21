package git

type Checkout interface {
	As(branch string) error
	OntoWorkspace() error
}
