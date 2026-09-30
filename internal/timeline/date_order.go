package timeline

import (
	"fmt"

	"github.com/chipskein/cade/internal/textnorm"
)

// DateOrder is how a numeric date without a year first ("12/08") is read.
// The zero value is DayFirst, Brazil's order and most locales'.
type DateOrder int

const (
	// DayFirst reads "12/08" as 12 August.
	DayFirst DateOrder = iota
	// MonthFirst reads "12/08" as December 8.
	MonthFirst
)

// dateOrderNames are the config's and the plan suite's spellings.
var dateOrderNames = map[string]DateOrder{"dmy": DayFirst, "mdy": MonthFirst}

// ParseDateOrder reads "dmy" or "mdy".
//
//	order, err := timeline.ParseDateOrder("mdy") // MonthFirst
func ParseDateOrder(name string) (DateOrder, error) {
	order, found := dateOrderNames[name]
	if !found {
		return DayFirst, fmt.Errorf("date order %q, expected \"dmy\" or \"mdy\"", name)
	}
	return order, nil
}

// dayAndMonth places the two numbers of a slash date by order.
func (order DateOrder) dayAndMonth(first, second int) (day, month int) {
	if order == MonthFirst {
		return second, first
	}
	return first, second
}

// HasNumericDate reports whether question holds a slash date ("12/08"),
// whose meaning depends on the DateOrder.
func HasNumericDate(question string) bool {
	return slashDatePattern.MatchString(textnorm.Fold(question))
}
