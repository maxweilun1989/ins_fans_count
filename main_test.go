package main

import (
	"bufio"
	"log"
	"os"
	"testing"
)

func TestGetStoriesLink(t *testing.T) {
	url := "https://www.instagram.com/jonatasbacciotti/"
	expected := "https://www.instagram.com/stories/jonatasbacciotti/"
	stories := getStoriesLink(url)
	if stories != expected {
		t.Errorf("Expected %s, got %s", expected, stories)
	}
}

func TestParseCount(t *testing.T) {
	file, err := os.Open("./assets/fans_count.txt")
	if err != nil {
		log.Fatalf("Can not open file, %v", err)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		count := parseFansCount(line)
		if count != 18302 {
			t.Errorf("Expected 18302, got %d", count)
		}
	}
}

func TestParseStoryLink(t *testing.T) {
	testParseStoryLink()
}
