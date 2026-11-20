package workflows

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

// deployInputs are the inputs of the storefront's Deploy workflow, one of each way of entering a
// value.
var deployInputs = []WorkflowInput{
	{Name: "environment", Description: "Where to deploy", Kind: InputKindChoice, Options: []string{"staging", "production"}, Default: "staging"},
	{Name: "size", Kind: InputKindChoice, Options: []string{"S", "M", "L"}, Default: "M"},
	{Name: "command", Description: "What to run", Required: true, Kind: InputKindText},
	{Name: "verbose", Kind: InputKindBoolean, Default: "true"},
	{Name: "replicas", Kind: InputKindNumber},
}

const (
	deploySection = `
[[workflows.sections]]
title = "Deployable"
kind = "workflows"
workflows = ["Deploy"]
`
	checksSection = `
[[workflows.sections]]
title = "Checks"
kind = "workflows"
workflows = ["Checks"]
`
)

func runsWithStatus(status string) string {
	return fmt.Sprintf("\n[[workflows.sections]]\ntitle = \"Runs\"\nstatus = [%q]\n", status)
}

// openedForm opens the form of the storefront's Deploy workflow.
func openedForm(t *testing.T, provider *fakeProvider) *plugin.Model {
	t.Helper()
	model := startedModel(t, provider, deploySection)
	plugintest.Press(t, model, "x")
	return model
}

func assertNothingStarted(t *testing.T, provider *fakeProvider) {
	t.Helper()
	if started := provider.startedWorkflows(); len(started) != 0 {
		t.Fatalf("started %v", started)
	}
}

func deployWorkflow(provider *fakeProvider) Workflow {
	return provider.workflows[storefrontRepository][0]
}

func TestRunKeyOnAWorkflowWithoutTheManualTriggerShowsANoticeAndOpensNoForm(t *testing.T) {
	provider := newFakeProvider()
	checks := provider.workflows[storefrontRepository][1]
	model := startedModel(t, provider, checksSection)

	plugintest.Press(t, model, "x")

	assertShows(t, model, checks.Name+" cannot be started by hand")
	assertHides(t, model, "Run "+checks.Name+" in", "ctrl+s")
}

func TestRunKeyOnAWorkflowWithTheManualTriggerOpensTheFormUnderAHeaderNamingTheRepository(t *testing.T) {
	provider := newFakeProvider()
	deploy := deployWorkflow(provider)

	model := openedForm(t, provider)

	assertShows(t, model, "Run "+deploy.Name+" in "+deploy.Repository.FullName())
}

func TestFormListsTheBranchAndThenTheInputsInDeclaredOrderWithTheirDefaults(t *testing.T) {
	provider := newFakeProvider()
	deploy := deployWorkflow(provider)

	model := openedForm(t, provider)

	labels := []string{branchLabel}
	for _, input := range deploy.Inputs {
		labels = append(labels, input.Name)
	}
	positions := order(model, labels...)
	for index := 1; index < len(positions); index++ {
		if positions[index-1] < 0 || positions[index] <= positions[index-1] {
			t.Fatalf("the labels %v are at %v:%s", labels, positions, describe(model))
		}
	}
	if line := lineWith(model, branchLabel); !strings.Contains(line, deploy.DefaultBranch) {
		t.Fatalf("the branch is drawn as %q", line)
	}
	if line := lineWith(model, "environment"); !strings.Contains(line, deploy.Inputs[0].Default) {
		t.Fatalf("the environment is drawn as %q", line)
	}
	if line := lineWith(model, "verbose"); !strings.Contains(line, "[x]") {
		t.Fatalf("the boolean is drawn as %q", line)
	}
}

func TestFormShowsTheDescriptionOfOnlyTheFocusedField(t *testing.T) {
	provider := newFakeProvider()
	environment := deployWorkflow(provider).Inputs[0]
	command := deployWorkflow(provider).Inputs[2]

	model := openedForm(t, provider)

	assertHides(t, model, environment.Description)

	plugintest.Press(t, model, "down")

	assertShows(t, model, environment.Description)
	assertHides(t, model, command.Description)
}

func TestLeftAndRightCycleTheOptionsOfTheFocusedChoiceAndWrapAround(t *testing.T) {
	provider := newFakeProvider()
	environment := deployWorkflow(provider).Inputs[0]

	model := openedForm(t, provider)
	plugintest.Press(t, model, "down", "right")

	if line := lineWith(model, environment.Name); !strings.Contains(line, environment.Options[1]) {
		t.Fatalf("after right the environment is drawn as %q", line)
	}

	plugintest.Press(t, model, "right")

	if line := lineWith(model, environment.Name); !strings.Contains(line, environment.Options[0]) {
		t.Fatalf("after wrapping the environment is drawn as %q", line)
	}

	plugintest.Press(t, model, "left")

	if line := lineWith(model, environment.Name); !strings.Contains(line, environment.Options[1]) {
		t.Fatalf("after left the environment is drawn as %q", line)
	}
}

func TestCyclingAChoiceLeavesTheOtherChoicesAlone(t *testing.T) {
	provider := newFakeProvider()
	size := deployWorkflow(provider).Inputs[1]

	model := openedForm(t, provider)
	plugintest.Press(t, model, "down", "right")

	if line := lineWith(model, size.Name); !strings.Contains(line, size.Default) {
		t.Fatalf("the size is drawn as %q", line)
	}
}

func TestHAndLCycleAChoiceButTypeIntoATextField(t *testing.T) {
	provider := newFakeProvider()
	environment := deployWorkflow(provider).Inputs[0]

	model := openedForm(t, provider)
	plugintest.Press(t, model, "l")

	if line := lineWith(model, branchLabel); !strings.Contains(line, deployWorkflow(provider).DefaultBranch+"l") {
		t.Fatalf("the branch is drawn as %q", line)
	}

	plugintest.Press(t, model, "down", "l")

	if line := lineWith(model, environment.Name); !strings.Contains(line, environment.Options[1]) {
		t.Fatalf("the environment is drawn as %q", line)
	}
}

func TestSpaceTogglesTheFocusedBooleanAndTypesASpaceIntoATextField(t *testing.T) {
	provider := newFakeProvider()

	model := openedForm(t, provider)
	plugintest.Press(t, model, "down", "down", "down", "down", " ")

	if line := lineWith(model, "verbose"); !strings.Contains(line, "[ ]") {
		t.Fatalf("the boolean is drawn as %q", line)
	}

	plugintest.Press(t, model, " ")

	if line := lineWith(model, "verbose"); !strings.Contains(line, "[x]") {
		t.Fatalf("the boolean is drawn as %q", line)
	}
}

func TestTypingEditsOnlyTheFocusedTextField(t *testing.T) {
	provider := newFakeProvider()
	command := deployWorkflow(provider).Inputs[2]

	model := openedForm(t, provider)
	plugintest.Press(t, model, "down", "down", "down", "make deploy")

	if line := lineWith(model, command.Name); !strings.Contains(line, "make deploy") {
		t.Fatalf("the command is drawn as %q", line)
	}
	if line := lineWith(model, branchLabel); strings.Contains(line, "make deploy") {
		t.Fatalf("the branch is drawn as %q", line)
	}
}

func TestQTypesIntoATextFieldInsteadOfClosingTheForm(t *testing.T) {
	model := openedForm(t, newFakeProvider())

	plugintest.Press(t, model, "q")

	assertShows(t, model, "ctrl+s")
}

func TestTabAndShiftTabMoveBetweenFieldsLikeTheArrowKeys(t *testing.T) {
	provider := newFakeProvider()
	environment := deployWorkflow(provider).Inputs[0]

	model := openedForm(t, provider)
	plugintest.Press(t, model, "tab")

	assertShows(t, model, environment.Description)

	plugintest.Press(t, model, "shift+tab")

	assertHides(t, model, environment.Description)
}

func TestSubmittingWithAnEmptyRequiredFieldMarksItAndDoesNotAsk(t *testing.T) {
	provider := newFakeProvider()
	deploy := deployWorkflow(provider)

	model := openedForm(t, provider)
	plugintest.Press(t, model, "ctrl+s")

	if line := lineWith(model, deploy.Inputs[2].Name); !strings.Contains(line, "required") {
		t.Fatalf("the command is drawn as %q", line)
	}
	if line := lineWith(model, branchLabel); strings.Contains(line, "required") {
		t.Fatalf("the branch is drawn as %q", line)
	}
	assertHides(t, model, "Run "+deploy.Name+" in "+deploy.Repository.Name+" on")
	plugintest.Press(t, model, "y")
	assertNothingStarted(t, provider)
}

func TestSubmittingWithAnEmptyBranchMarksTheBranch(t *testing.T) {
	provider := newFakeProvider()
	deploy := deployWorkflow(provider)

	model := openedForm(t, provider)
	for range deploy.DefaultBranch {
		plugintest.Send(t, model, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	plugintest.Press(t, model, "ctrl+s")

	if line := lineWith(model, branchLabel); !strings.Contains(line, "required") {
		t.Fatalf("the branch is drawn as %q", line)
	}
}

func TestSubmittingANumberFieldThatIsNotANumberMarksIt(t *testing.T) {
	provider := newFakeProvider()
	replicas := deployWorkflow(provider).Inputs[4]

	model := openedForm(t, provider)
	plugintest.Press(t, model, "down", "down", "down", "make deploy", "down", "down", "three", "ctrl+s")

	if line := lineWith(model, replicas.Name); !strings.Contains(line, "must be a number") {
		t.Fatalf("the replicas are drawn as %q", line)
	}
	assertHides(t, model, "make deploy?")
}

// fillInCommand types the one required value of the deploy form.
func fillInCommand(t *testing.T, model *plugin.Model) {
	t.Helper()
	plugintest.Press(t, model, "down", "down", "down", "make deploy", "ctrl+s")
}

func TestSubmittingAsksToConfirmWithTheValuesInTheQuestionAndStartsNothingYet(t *testing.T) {
	provider := newFakeProvider()
	deploy := deployWorkflow(provider)

	model := openedForm(t, provider)
	fillInCommand(t, model)

	question := fmt.Sprintf("Run %s in %s on %s with environment=%s, size=%s, command=make deploy, verbose=true?",
		deploy.Name, deploy.Repository.Name, deploy.DefaultBranch, deploy.Inputs[0].Default, deploy.Inputs[1].Default)
	assertShows(t, model, question)
	assertHides(t, model, "replicas=", "ctrl+s")
	assertNothingStarted(t, provider)
}

func TestConfirmingStartsTheWorkflowOnTheBranchWithTheEnteredValues(t *testing.T) {
	provider := newFakeProvider()
	deploy := deployWorkflow(provider)

	model := openedForm(t, provider)
	fillInCommand(t, model)
	plugintest.Press(t, model, "y")

	started := provider.startedWorkflows()
	if len(started) != 1 || started[0].workflow.File != deploy.File || started[0].branch != deploy.DefaultBranch {
		t.Fatalf("started %+v", started)
	}
	expected := map[string]string{
		deploy.Inputs[0].Name: deploy.Inputs[0].Default, deploy.Inputs[1].Name: deploy.Inputs[1].Default,
		deploy.Inputs[2].Name: "make deploy", deploy.Inputs[3].Name: "true", deploy.Inputs[4].Name: "",
	}
	for name, value := range expected {
		if started[0].inputs[name] != value {
			t.Fatalf("inputs = %v, expected %v", started[0].inputs, expected)
		}
	}
}

func TestDecliningTheConfirmationStartsNothing(t *testing.T) {
	provider := newFakeProvider()

	model := openedForm(t, provider)
	fillInCommand(t, model)
	plugintest.Press(t, model, "n")

	assertNothingStarted(t, provider)
}

func TestSubmittingClosesTheForm(t *testing.T) {
	model := openedForm(t, newFakeProvider())

	fillInCommand(t, model)

	assertHides(t, model, "ctrl+s run")
}

func TestEscapeClosesTheFormWithoutStartingAnything(t *testing.T) {
	provider := newFakeProvider()

	model := openedForm(t, provider)
	plugintest.Press(t, model, "down", "down", "down", "make deploy", "esc", "y")

	assertHides(t, model, "ctrl+s run")
	assertNothingStarted(t, provider)
}

func TestFormHelpNamesTheKeysOfTheFocusedFieldOnly(t *testing.T) {
	model := openedForm(t, newFakeProvider())

	assertHides(t, model, "option", "toggle")

	plugintest.Press(t, model, "down")

	assertShows(t, model, "←/→ option")
	assertHides(t, model, "toggle")

	plugintest.Press(t, model, "down", "down", "down")

	assertShows(t, model, "space toggle")
	assertHides(t, model, "option")
}
