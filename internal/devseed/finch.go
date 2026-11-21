// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package devseed

import "wiekko.net/internal/model"

// seedFinchTracker builds the contrast fixture: a small kanban product with
// estimation switched off, so both workflow modes are represented without
// having to reconfigure Aviary. No sprints, no campaigns — deliberately thin.
func (s *seeder) seedFinchTracker() {
	p := s.newProduct(
		"Finch Tracker",
		"Ringing station companion: log ringed birds, recaptures and biometrics.",
		model.EstimationModeDisabled,
		model.WorkflowModeKanban,
	)

	p.member("bo", model.RoleTester)

	p.milestone("Season 1", "First ringing season in production use.", model.MilestoneTypeRelease, day(45))

	p.environment("Tablet (Android)")

	p.component("Ringing Log", "Recording ring numbers, species and biometrics at the station.", "Season 1")
	p.component("Recapture Matching", "Matching a recaptured bird against earlier records.", "Season 1")
	p.component("Reporting", "Season summaries and exports for the national scheme.", "Season 1")

	p.capability("Ringing Log", "Ring Entry", "Enter a ring number and the bird's biometrics.", "Season 1")
	p.capability("Recapture Matching", "Ring Lookup", "Find every earlier record for a ring number.", "Season 1")
	p.capability("Reporting", "Scheme Export", "Export a season in the national scheme's exchange format.", "Season 1")

	p.requirement(reqSpec{
		handle:      "ring-entry",
		category:    "Functionality",
		title:       "Record a ringed bird",
		description: "Ring number, species, age, sex and the standard biometrics.",
		status:      "In Progress",
		milestone:   "Season 1",
		priority:    1,
		active:      true,
		capabilties: []string{"Ring Entry"},
	})
	p.requirement(reqSpec{
		handle:      "ring-validate",
		parent:      "ring-entry",
		category:    "Functionality",
		title:       "Validate ring numbers against the issued series",
		description: "A ring number outside the station's issued series is rejected at entry.",
		dod:         "Entering a ring outside the series shows an inline error and blocks the save.",
		status:      "New",
		milestone:   "Season 1",
		priority:    2,
		capabilties: []string{"Ring Entry"},
	})
	p.requirement(reqSpec{
		handle:      "recapture",
		category:    "Functionality",
		title:       "Match a recapture to its earlier records",
		description: "Entering a known ring number surfaces every previous capture of that bird.",
		status:      "In Progress",
		milestone:   "Season 1",
		priority:    1,
		active:      true,
		capabilties: []string{"Ring Lookup"},
	})
	p.requirement(reqSpec{
		handle:      "biometrics",
		category:    "Functionality",
		title:       "Capture wing, weight and fat score",
		status:      "Done",
		milestone:   "Season 1",
		priority:    2,
		active:      true,
		capabilties: []string{"Ring Entry"},
	})
	p.requirement(reqSpec{
		handle:      "export",
		category:    "Functionality",
		title:       "Export a season for the national scheme",
		description: "Produce the scheme's exchange format for a date range, ready to submit.",
		status:      "New",
		milestone:   "Season 1",
		priority:    3,
		coarse:      true,
		capabilties: []string{"Scheme Export"},
	})
	p.requirement(reqSpec{
		category:    "Architecture",
		title:       "Runs on the station tablet with no connectivity",
		description: "The ringing hut has no signal; everything is local until the day's data is synced.",
		status:      "Done",
		milestone:   "Season 1",
		priority:    1,
		active:      true,
	})
	p.requirement(reqSpec{
		handle:      "sec-export",
		category:    "Security",
		title:       "Exports carry no ringer personal data",
		description: "The scheme file identifies the station, not the individual ringer.",
		status:      "New",
		milestone:   "Season 1",
		priority:    2,
		capabilties: []string{"Scheme Export"},
	})
	p.requirement(reqSpec{
		kind:      model.RequirementKindTask,
		category:  "Tooling",
		title:     "Package the tablet build for side-loading",
		status:    "In Review",
		milestone: "Season 1",
		priority:  3,
		active:    true,
	})

	p.constraint(constraintSpec{
		category:    "Architecture",
		title:       "Single-file local database, copyable off the tablet",
		description: "A ringer must be able to hand the season's data over as one file.",
		compliance:  model.ComplianceMet,
		milestone:   "Season 1",
		priority:    1,
	})
	p.constraint(constraintSpec{
		category:    "UX",
		title:       "Entry is possible with wet hands and no stylus",
		description: "Ringing happens outdoors in the rain more often than not.",
		compliance:  model.ComplianceNotAssessed,
		milestone:   "Season 1",
		priority:    2,
		linkCaps:    []string{"Ring Entry"},
	})

	p.defect(defectSpec{
		handle:      "ring-leading-zero",
		title:       "Leading zeros are stripped from ring numbers",
		description: "Ring 0042 is stored as 42, which then fails to match on recapture.",
		criticality: model.DefectCriticalityHigh,
		status:      "In Progress",
		milestone:   "Season 1",
		environment: "Tablet (Android)",
		priority:    1,
		active:      true,
		linkReqs:    []string{"ring-entry", "recapture"},
		linkCaps:    []string{"Ring Entry"},
	})
	p.defect(defectSpec{
		handle:      "weight-decimal",
		title:       "Weight field rejects a decimal comma",
		description: "The tablet's locale uses a comma; the field only accepts a period.",
		criticality: model.DefectCriticalityMedium,
		status:      "New",
		milestone:   "Season 1",
		environment: "Tablet (Android)",
		priority:    2,
		linkReqs:    []string{"biometrics"},
	})
	p.defect(defectSpec{
		handle:      "export-encoding",
		title:       "Export writes UTF-8 where the scheme expects Latin-1",
		description: "Species names with diacritics come back rejected by the submission portal.",
		criticality: model.DefectCriticalityLow,
		status:      "New",
		milestone:   "Season 1",
		priority:    3,
		linkReqs:    []string{"export"},
		linkComps:   []string{"Reporting"},
	})

	p.testCase(testCaseSpec{
		title:      "Ring numbers keep their leading zeros",
		component:  "Ringing Log",
		capability: "Ring Entry",
		level:      "Unit",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestRingNumber_PreservesLeadingZeros",
		reqs:       []string{"ring-entry"},
	})
	p.testCase(testCaseSpec{
		title:      "Recapture lookup returns every earlier record",
		component:  "Recapture Matching",
		capability: "Ring Lookup",
		level:      "Integration",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestRecapture_ReturnsFullHistory",
		envs:       []string{"Tablet (Android)"},
		reqs:       []string{"recapture"},
	})
	p.testCase(testCaseSpec{
		title:       "Log a full ringing session on the tablet",
		description: "Thirty birds, mixed new captures and recaptures, no connectivity.",
		component:   "Ringing Log",
		capability:  "Ring Entry",
		level:       "End-to-End",
		mode:        model.ExecutionModeManual,
		envs:        []string{"Tablet (Android)"},
		reqs:        []string{"ring-entry", "recapture"},
	})

	p.comment("bo", model.OwnerTypeDefect, "ring-leading-zero",
		"This one bit us on real data — three recaptures went unmatched before anyone noticed the ring numbers were short.")
}
