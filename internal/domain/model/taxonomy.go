package model

import "fmt"

type ExpenseType struct {
	year     int
	code     string
	label    string
	category ExpenseCategory
}

func NewExpenseType(year int, code, label string, cat ExpenseCategory) (ExpenseType, error) {
	if code == "" {
		return ExpenseType{}, fmt.Errorf("expense type code must not be empty")
	}
	if _, err := ParseExpenseCategory(string(cat)); err != nil {
		return ExpenseType{}, err
	}
	return ExpenseType{year, code, label, cat}, nil
}

func (t ExpenseType) Year() int                 { return t.year }
func (t ExpenseType) Code() string              { return t.code }
func (t ExpenseType) Label() string             { return t.label }
func (t ExpenseType) Category() ExpenseCategory { return t.category }

type ExpenseSubtype struct {
	year     int
	code     string
	label    string
	typeCode string
}

func NewExpenseSubtype(year int, code, label, typeCode string) (ExpenseSubtype, error) {
	if code == "" {
		return ExpenseSubtype{}, fmt.Errorf("expense subtype code must not be empty")
	}
	if typeCode == "" {
		return ExpenseSubtype{}, fmt.Errorf("expense subtype typeCode must not be empty")
	}
	return ExpenseSubtype{year, code, label, typeCode}, nil
}

func (s ExpenseSubtype) Year() int        { return s.year }
func (s ExpenseSubtype) Code() string     { return s.code }
func (s ExpenseSubtype) Label() string    { return s.label }
func (s ExpenseSubtype) TypeCode() string { return s.typeCode }
