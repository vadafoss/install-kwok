package constants

type Action string

const (
	Owner             = "kubernetes-sigs"
	Repo              = "kwok"
	Apply      Action = "apply"
	Delete     Action = "delete"
	MinVersion        = "v0.4.0"
	KwokRepo          = "kubernetes-sigs/kwok"
	CrbName           = "kwok-provider"
)
