package stubs

var WorldHandler = "WorldOps.CalculateWorld"
var KillHandler = "WorldOps.KillWorker"

type WorldReq struct {
	World    [][]byte
	Width    int
	Height   int
	StartRow int
	EndRow   int
}

type WorldRes struct {
	World [][]byte
}
