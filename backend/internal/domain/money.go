package domain

const MaxMoney int64 = 9007199254740991

func CalculationOverflow() error {
	return &DomainError{Code: "CALCULATION_OVERFLOW", Message: "financial calculation exceeds the supported money range", Err: ErrConflict}
}

func AddMoney(a, b int64) (int64, error) {
	if a > MaxMoney || a < -MaxMoney || b > MaxMoney || b < -MaxMoney {
		return 0, CalculationOverflow()
	}
	v := a + b
	if v > MaxMoney || v < -MaxMoney {
		return 0, CalculationOverflow()
	}
	return v, nil
}
