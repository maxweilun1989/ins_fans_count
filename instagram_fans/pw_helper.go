package instagram_fans

import (
	"fmt"
	"github.com/playwright-community/playwright-go"
	"strings"
	"time"
)

type Condition interface {
	Wait(page *playwright.Page, timeout float64) (bool, error)
}

type ElementCondition struct {
	Selector string
}

func (ec ElementCondition) Wait(page *playwright.Page, timeout float64) (bool, error) {
	mileSecond := timeout * 1000
	ele, err := (*page).WaitForSelector(ec.Selector, playwright.PageWaitForSelectorOptions{
		Timeout: &mileSecond,
	})
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

func (tc TextCondition) Wait(page *playwright.Page, timeout float64) (bool, error) {
	time.Sleep(time.Duration(10) * time.Second)
	pageContent, err := (*page).Content()
	if err != nil {
		return false, err
	}

	if strings.Contains(pageContent, tc.Text) {
		return true, nil
	}
	return false, nil
}

type StatusCondition struct {
	State *playwright.LoadState
}

func (sc StatusCondition) String() string {
	return fmt.Sprintf("StatusCondition{State: %v}", sc.State)
}

func (sc StatusCondition) Wait(page *playwright.Page, timeout float64) (bool, error) {
	err := (*page).WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   sc.State,
		Timeout: &timeout,
	})
	if err != nil {
		return false, err
	}
	time.Sleep(time.Duration(5) * time.Second)
	return true, nil
}

type TimeCondition struct {
	Time float64
}

func (tc TimeCondition) Wait(page *playwright.Page) error {
	(*page).WaitForTimeout(tc.Time)
	return nil
}

func WaitForConditions(page *playwright.Page, conditions []Condition, timeout float64) (Condition, error) {
	resultChan := make(chan Condition)
	errChan := make(chan error)

	for _, condition := range conditions {
		go func(cond Condition) {
			result, err := cond.Wait(page, timeout)
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
