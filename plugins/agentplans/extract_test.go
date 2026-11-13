package agentplans

import (
	"reflect"
	"testing"
)

func TestPullRequestNumbersAreFoundAfterAHashAndInAPullAddress(t *testing.T) {
	text := "Fixes #123, see also (#98) and https://github.com/acme/storefront/pull/45.\n#7 starts a line."

	numbers := pullRequestNumbers(text)

	if expected := []int{123, 98, 45, 7}; !reflect.DeepEqual(numbers, expected) {
		t.Fatalf("numbers = %v, expected %v", numbers, expected)
	}
}

func TestPullRequestNumbersIgnoreWhatOnlyLooksLikeAHash(t *testing.T) {
	for _, text := range []string{"&#123;", "abc#12", "# 12 Heading", "see /issues/#12", "#12abc", "#", "no numbers"} {
		if numbers := pullRequestNumbers(text); len(numbers) != 0 {
			t.Errorf("%q gave %v", text, numbers)
		}
	}
}

func TestPullRequestNumbersCountEachOnceWhicheverWayItIsMentioned(t *testing.T) {
	text := "#12 and #12 again, and acme/storefront/pull/12 and #30"

	numbers := pullRequestNumbers(text)

	if expected := []int{30, 12}; !reflect.DeepEqual(numbers, expected) {
		t.Fatalf("numbers = %v, expected %v", numbers, expected)
	}
}
