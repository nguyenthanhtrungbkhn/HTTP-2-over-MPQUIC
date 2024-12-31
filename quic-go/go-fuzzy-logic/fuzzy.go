// Package fuzzy is a package that implement fuzzy logic to BLT decider.
package fuzzy

import "math"


//cwndlevel			
const(				
	MustBeLowDebt = 0.2
	MustNotBeLowDebt = 0.4		
				
	MustNotBeMiddleLowDebt = 0.2		
	MustBeMiddleLowDebt = 0.4			
	MustBeMiddleHighDebt = 0.6		
	MustNotBeMiddleHighDebt = 0.9			
				
	MustNotBeHighDebt = 0.6	
	MustBeHighDebt = 0.9	
)	

//vrtt
// const(				
// 	MustBeLowIncome = 0.05			
// 	MustNotBeLowIncome = 0.1			
				
// 	MustNotBeMiddleLowIncome = 0.05			
// 	MustBeMiddleLowIncome = 0.1		
// 	MustBeMiddleHighIncome = 0.2			
// 	MustNotBeMiddleHighIncome = 0.3		
				
// 	MustNotBeHighIncome = 0.2			
// 	MustBeHighIncome = 0.3			
// )		

const(				
	MustBeLowIncome = 0.1			
	MustNotBeLowIncome = 0.3			
				
	MustNotBeMiddleLowIncome = 0.1			
	MustBeMiddleLowIncome = 0.3	
	MustBeMiddleHighIncome = 0.4			
	MustNotBeMiddleHighIncome = 0.6		
				
	MustNotBeHighIncome = 0.4			
	MustBeHighIncome = 0.6			
)				


const(
	RejectedValue = 0.3
	ConsideredValue = 0.5
	AcceptedValue = 0.7
)

// Fuzzy is the main interface of a fuzzy logic algorithm
type Fuzzy interface {
	Fuzzification(number *FuzzyNumber) error
	Defuzzification(number *FuzzyNumber) error
	Inference(number *FuzzyNumber) error
}

// Family is the data needed for this programe.
type Family struct {
	Number string
	Income float64
	Debt float64
}

// FuzzyNumber is the struct thatwill continually holds any fuzzy data.
type FuzzyNumber struct {
	Family Family

	IncomeMembership []float64
	DebtMembership []float64

	AccepetedInference float64
	ConsideredInference float64
	RejectedInference float64

	CrispValue float64
}

type BLT struct {
}
// Inference is a function that change from raw linguistic into fuzzy linguistic.
func (b *BLT) Inference(number *FuzzyNumber) error {
	number.RejectedInference =  math.Max(math.Min(number.IncomeMembership[0], number.DebtMembership[0]), math.Min(number.IncomeMembership[1], number.DebtMembership[0]))
	number.RejectedInference = math.Max(number.RejectedInference, math.Min(number.IncomeMembership[0], number.DebtMembership[1]))

	number.ConsideredInference = math.Max(math.Min(number.IncomeMembership[2], number.DebtMembership[0]), math.Min(number.IncomeMembership[1], number.DebtMembership[1]))
	number.ConsideredInference = math.Max(number.ConsideredInference, math.Min(number.IncomeMembership[0], number.DebtMembership[2]))
  
  	number.AccepetedInference = math.Max(math.Min(number.IncomeMembership[2], number.DebtMembership[1]), math.Min(number.IncomeMembership[1], number.DebtMembership[2]))
	number.AccepetedInference = math.Max(number.AccepetedInference, math.Min(number.IncomeMembership[2], number.DebtMembership[2]))
 
    return nil
}






// Defuzzification is a function that will transfer fuzzy linguistic to crisp data.
func (b *BLT) Defuzzification(number *FuzzyNumber) error {
	number.CrispValue = 0
	number.CrispValue += number.AccepetedInference*AcceptedValue
	number.CrispValue += number.ConsideredInference* ConsideredValue
	number.CrispValue += number.RejectedInference* RejectedValue
	number.CrispValue /= (number.AccepetedInference+number.ConsideredInference+number.RejectedInference)

	return nil
}

// Fuzzification is a function that will transfer crisp data into linguistic.
func (b *BLT) Fuzzification(number *FuzzyNumber) error {
	number.DebtMembership = append(number.DebtMembership, b.DebtLow(number.Family.Debt))
	number.DebtMembership = append(number.DebtMembership, b.DebtMiddle(number.Family.Debt))
	number.DebtMembership = append(number.DebtMembership, b.DebtHigh(number.Family.Debt))

	number.IncomeMembership = append(number.IncomeMembership, b.IncomeLow(number.Family.Income))
	number.IncomeMembership = append(number.IncomeMembership, b.IncomeMiddle(number.Family.Income))
	number.IncomeMembership = append(number.IncomeMembership, b.IncomeHigh(number.Family.Income))
	return nil
}

func (b *BLT) IncomeLow(income float64) float64 {
	if income <= MustBeLowIncome {
		return 1
	} else if income > MustNotBeLowIncome {
		return 0
	}
	return 1 - (float64(income - MustBeLowIncome) / float64(MustNotBeLowIncome - MustBeLowIncome))
}

func (b *BLT) IncomeMiddle(income float64) float64 {
	if income > MustBeMiddleLowIncome && income <= MustBeMiddleHighIncome {
		return 1
	} else if income < MustNotBeMiddleLowIncome || income > MustNotBeMiddleHighIncome {
		return 0
	} else if income < MustBeMiddleLowIncome && income >= MustNotBeMiddleLowIncome {
		return float64(income - MustNotBeMiddleLowIncome) / float64(MustBeMiddleLowIncome - MustNotBeMiddleLowIncome)
	}

	return 1 - float64(income - MustBeMiddleHighIncome) / float64(MustNotBeMiddleHighIncome - MustBeMiddleHighIncome)
}

func (b *BLT) IncomeHigh(income float64) float64 {
	if income <= MustNotBeHighIncome {
		return 0
	} else if income > MustBeHighIncome {
		return 1
	}
	return float64(income - MustNotBeHighIncome) / float64(MustBeHighIncome - MustNotBeHighIncome)
}


func (b *BLT) DebtLow(income float64) float64 {
	if income <= MustBeLowDebt {
		return 1
	} else if income > MustNotBeLowDebt {
		return 0
	}
	return 1-(float64(income - MustBeLowDebt) / float64(MustNotBeLowDebt - MustBeLowDebt))
}

func (b *BLT) DebtMiddle(income float64) float64 {
	if income > MustBeMiddleLowDebt && income <= MustBeMiddleHighDebt {
		return 1
	} else if income < MustNotBeMiddleLowDebt || income > MustNotBeMiddleHighDebt {
		return 0
	} else if income < MustBeMiddleLowDebt && income >= MustNotBeMiddleLowDebt {
		return float64(income - MustNotBeMiddleLowDebt) / float64(MustBeMiddleLowDebt - MustNotBeMiddleLowDebt)
	}

	return 1 - (float64(income - MustBeMiddleHighDebt) / float64(MustNotBeMiddleHighDebt - MustBeMiddleHighDebt))
}

func (b *BLT) DebtHigh(income float64) float64 {
	if income <= MustNotBeHighDebt {
		return 0
	} else if income > MustBeHighDebt {
		return 1
	}
	return float64(income - MustNotBeHighDebt) / float64(MustBeHighDebt - MustNotBeHighDebt)
}