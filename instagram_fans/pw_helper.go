package instagram_fans

import (
	"github.com/playwright-community/playwright-go"
	"strings"
)

type Condition interface {
	Wait(page *playwright.Page) (bool, error)
}

type ElementCondition struct {
	Selector string
}

func (ec ElementCondition) Wait(page *playwright.Page) (bool, error) {
	ele, err := (*page).WaitForSelector(ec.Selector)
	if err != nil {
		return false, err
	}
	if ele != nil {
		return true, nil
	}
	return false, nil
}

type TextCondition struct {
	Text string
}

func (tc TextCondition) Wait(page *playwright.Page) (bool, error) {
	pageContent, err := (*page).Content()
	if err != nil {
		return false, err
	}

	if strings.Contains(pageContent, tc.Text) {
		return true, nil
	}
	return false, nil
}

type TimeCondition struct {
	Time float64
}

func (tc TimeCondition) Wait(page *playwright.Page) error {
	(*page).WaitForTimeout(tc.Time)
	return nil
}

func WaitForConditions(page *playwright.Page, conditions []Condition) (Condition, error) {
	resultChan := make(chan Condition)
	errChan := make(chan error)

	for _, condition := range conditions {
		go func(cond Condition) {
			result, err := cond.Wait(page)
			if err != nil {
				errChan <- err
			} else {
				if result {
					resultChan <- cond
				}
			}
		}(condition)
	}

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return nil, err
	}
}
