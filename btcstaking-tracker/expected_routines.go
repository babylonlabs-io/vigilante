package btcstakingtracker

type IBTCSlasher interface {
	Bootstrap(startHeight uint64) error
	LastEvidencesHeight() (uint64, bool, error)
	Start() error
	Stop() error
}

type IAtomicSlasher interface {
	Start() error
	Stop() error
}
