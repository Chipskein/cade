package retrievalsuite

import (
	"fmt"
	"io"
	"math"
)

// Calibration summarizes, for questions without filters (the only ones
// the distance gates judge), how near their closest event was. Run the
// calibration set with the gates off, so every closest event is seen.
type Calibration struct {
	Answerable      int
	Unanswerable    int
	AnswerableMax   float64
	UnanswerableMin float64
}

// Calibrate reads the closest distance of each unscoped case.
//
//	calibration := retrievalsuite.Calibrate(board)
func Calibrate(board Scoreboard) Calibration {
	calibration := Calibration{AnswerableMax: math.Inf(-1), UnanswerableMin: math.Inf(1)}
	for _, result := range board.Results {
		if result.Scoped || len(result.Distances) == 0 {
			continue
		}
		closest := result.Distances[0]
		if result.Answerable() {
			calibration.Answerable++
			calibration.AnswerableMax = max(calibration.AnswerableMax, closest)
		} else {
			calibration.Unanswerable++
			calibration.UnanswerableMin = min(calibration.UnanswerableMin, closest)
		}
	}
	return calibration
}

// Separates reports whether one max_best_distance keeps every answerable
// question and rejects every unanswerable one.
func (c Calibration) Separates() bool {
	return c.AnswerableMax < c.UnanswerableMin
}

// Suggested is the midpoint between the two groups.
func (c Calibration) Suggested() float64 {
	return (c.AnswerableMax + c.UnanswerableMin) / 2
}

// WriteReport prints the two groups and the suggested gate next to the
// configured one; the configuration is not changed automatically.
func (c Calibration) WriteReport(out io.Writer, configured float64) {
	fmt.Fprintf(out, "calibração (perguntas sem filtro, distância do evento mais próximo):\n")
	fmt.Fprintf(out, "  com resposta: %d, pior %.3f\n  sem resposta: %d, melhor %.3f\n", c.Answerable, c.AnswerableMax, c.Unanswerable, c.UnanswerableMin)
	if !c.Separates() {
		fmt.Fprintf(out, "  os grupos se sobrepõem: nenhum max_best_distance separa os dois (atual %.2f)\n", configured)
		return
	}
	fmt.Fprintf(out, "  max_best_distance sugerido %.3f (atual %.2f)\n", c.Suggested(), configured)
}
