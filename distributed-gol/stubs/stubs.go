package stubs

import "uk.ac.bris.cs/gameoflife/util"

var EvolveWorldHandler = "Broker.EvolveWorld"
var AliveCellsCountHandler = "Broker.AliveCellsCount"
var AliveCellsHandler = "Broker.CalculateAliveCells"
var GetGlobalHandler = "Broker.GetGlobal"
var PauseHandler = "Broker.Pause"
var UnpauseHandler = "Broker.Unpause"
var QuitHandler = "Broker.QuitServer"
var KillServerHandler = "Broker.KillServer"
var GetBrokerCellFlippedHandler = "Broker.GetCellFlipped"
var GetTurnDoneHandler = "Broker.GetTurnDone"
var GetContinueHandler = "Broker.GetContinue"

type EvolveResponse struct {
	World [][]byte
	Turn  int
}

type EvolveWorldRequest struct {
	World       [][]byte
	Width       int
	Height      int
	Turn        int
	Threads     int
	ImageHeight int
	ImageWidth  int
}
type CalculateAliveCellsRequest struct {
	World [][]byte
}
type CalculateAliveCellsResponse struct {
	AliveCells []util.Cell
}
type AliveCellsCountResponse struct {
	AliveCellsCount int
	CompletedTurns  int
}
type GetGlobalResponse struct {
	World [][]byte
	Turns int
}
type Empty struct{}

type GetBrokerCellFlippedResponse struct {
	FlippedEvents []FlippedEvent
}

type GetTurnDoneResponse struct {
	TurnDone bool
	Turn     int
}

type GetContinueResponse struct {
	Continue bool
	World    [][]byte
	Turn     int
}
type FlippedEvent struct {
	CompletedTurns int
	Cell           util.Cell
}
