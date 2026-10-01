package presentation

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func exploreTestGraphs(revision string) (DemoGraph, DemoGraph) {
	limits := demoGraphLimits{views: 30, perScreen: 6, depth: 5}
	teacher := exploreDemoGraph("teacher", "Teacher", func() dashboardModel { return demoExploreModel(revision, 120, 30, true) }, limits)
	administrator := exploreDemoGraph("administrator", "Administrator", func() dashboardModel { return demoExploreModel(revision, 120, 30, false) }, limits)
	return teacher, administrator
}

func demoGraphText(graph DemoGraph, view int) string {
	lines := make([]string, len(graph.Views[view].Lines))
	for position, index := range graph.Views[view].Lines {
		lines[position] = demoANSI.ReplaceAllString(graph.Lines[index], "")
	}
	return strings.Join(lines, "\n")
}

func TestDemoGraphsNavigateTheRealDashboard(t *testing.T) {
	t.Parallel()
	revision := strings.Repeat("a", 40)
	teacher, administrator := exploreTestGraphs(revision)
	for _, graph := range []DemoGraph{teacher, administrator} {
		if len(graph.Views) < 10 {
			t.Fatalf("%s graph explored only %d views", graph.ID, len(graph.Views))
		}
		for id, view := range graph.Views {
			for key, next := range view.Next {
				if next < 0 || next >= len(graph.Views) || next == id {
					t.Fatalf("%s view %d key %s points to %d", graph.ID, id, key, next)
				}
			}
			for _, line := range view.Lines {
				if line < 0 || line >= len(graph.Lines) {
					t.Fatalf("%s view %d references missing line %d", graph.ID, id, line)
				}
			}
		}
	}
	if start := demoGraphText(teacher, teacher.Start); !strings.Contains(start, "Classroom controls") || !strings.Contains(start, "Power controls") {
		t.Fatalf("teacher graph does not start on the classroom dashboard:\n%s", start)
	}
	for id := range teacher.Views {
		if screen := demoGraphScreen(demoGraphText(teacher, id)); strings.Contains(screen, "Maintenance") || strings.Contains(screen, "Software") {
			t.Fatalf("teacher graph reached administration: %s", screen)
		}
	}
	software, ok := administrator.Views[administrator.Start].Next["w"]
	if !ok || !strings.Contains(demoGraphText(administrator, software), "Software") {
		t.Fatal("administrator graph does not open Software from the overview")
	}
	again, _ := exploreTestGraphs(revision)
	if !reflect.DeepEqual(teacher, again) {
		t.Fatal("demo graph exploration is not deterministic")
	}
}

func TestDemoGraphsStopAtTypedConfirmations(t *testing.T) {
	t.Parallel()
	revision := strings.Repeat("a", 40)
	var applied atomic.Int32
	for _, classroom := range []bool{true, false} {
		start := func() dashboardModel {
			model := demoExploreModel(revision, 120, 30, classroom)
			model.actions.ApplyShutdown = func(domain.ShutdownPlanReport) domain.ShutdownApplyReport {
				applied.Add(1)
				return domain.ShutdownApplyReport{}
			}
			model.actions.ApplyInternet = func(domain.InternetPlan) domain.InternetReport { applied.Add(1); return domain.InternetReport{} }
			model.actions.ApplyDeployment = func(context.Context, domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
				applied.Add(1)
				return domain.DeploymentExecutionReport{}
			}
			return model
		}
		exploreDemoGraph("probe", "Probe", start, demoGraphLimits{views: 50, perScreen: 10, depth: 6})
	}
	if count := applied.Load(); count != 0 {
		t.Fatalf("explorer applied %d reviewed operations without their typed confirmation", count)
	}
}
