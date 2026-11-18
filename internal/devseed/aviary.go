package devseed

import "nexmap.net/internal/model"

// seedAviary builds the main demo fixture: a bird-watching logbook app run in
// scrum mode with story points. It is deliberately the fuller of the two
// products — it exercises the sprint board, the backlog tree, the traceability
// links and the campaign result matrix.
func (s *seeder) seedAviary() {
	p := s.newProduct(
		"Aviary",
		"Field logbook for birders: record sightings offline, look up species, and share a life list.",
		model.EstimationModeStoryPoints,
		model.WorkflowModeScrum,
	)

	// Members first — the campaign results below are recorded by bo, who needs
	// the tester role before SetResultStatus will accept them.
	p.member("ada", model.RoleViewer)
	p.member("bo", model.RoleTester)
	p.member("cy", model.RoleDeveloper)

	s.aviaryPlanning(p)
	s.aviaryArchitecture(p)
	s.aviaryBacklog(p)
	s.aviaryConstraints(p)
	s.aviaryDefects(p)
	s.aviaryTestCases(p)
	s.aviaryDocuments(p)
	s.aviaryCampaigns(p)
	s.aviaryDiscussion(p)
}

func (s *seeder) aviaryPlanning(p *productSeed) {
	p.milestone("0.1 Field Trial", "Closed beta with the local ornithological society.", model.MilestoneTypeInternal, day(-30))
	p.milestone("1.0 Public Launch", "App store release, iOS and Android.", model.MilestoneTypeRelease, day(60))
	p.milestone("1.1 Sharing", "Public life lists and follower feeds.", model.MilestoneTypePlanning, day(150))

	// Ordering matters: at most one sprint may be active, so Sprint 12 is
	// completed before Sprint 13 is started.
	p.sprint("Sprint 12", "0.1 Field Trial", day(-28), day(-14), model.SprintStatusCompleted)
	p.sprint("Sprint 13", "1.0 Public Launch", day(-7), day(7), model.SprintStatusActive)
	p.sprint("Sprint 14", "1.0 Public Launch", day(7), day(21), model.SprintStatusPlanned)

	p.environment("iOS 17")
	p.environment("Android 14")
	p.environment("Web (Chrome)")
}

func (s *seeder) aviaryArchitecture(p *productSeed) {
	p.component("Field Logging", "Everything that happens while standing in a marsh with cold hands.", "0.1 Field Trial")
	p.component("Species Catalogue", "Taxonomy, regional lists and identification hints.", "0.1 Field Trial")
	p.component("Sync & Storage", "Offline-first local store and conflict resolution against the server.", "1.0 Public Launch")
	p.component("Sharing", "Publishing observations and life lists to other birders.", "1.1 Sharing")

	p.capability("Field Logging", "Quick Entry", "Record a sighting in under five seconds, one-handed.", "0.1 Field Trial")
	p.capability("Field Logging", "Photo Attachment", "Attach photos from the camera or library to a sighting.", "1.0 Public Launch")
	p.capability("Species Catalogue", "Species Search", "Find a species by name, by region, or by narrowing down traits.", "0.1 Field Trial")
	p.capability("Sync & Storage", "Offline Sync", "Queue observations offline and reconcile them once a connection returns.", "1.0 Public Launch")
	p.capability("Sharing", "Public Life List", "Publish a life list under a shareable link.", "1.1 Sharing")
}

func (s *seeder) aviaryBacklog(p *productSeed) {
	// A coarse parent with children, so the backlog tree has something to fold.
	p.requirement(reqSpec{
		handle:      "sighting",
		category:    "Functionality",
		title:       "Record a sighting",
		description: "A birder can record what they saw, where, and when, without leaving the field.",
		status:      "In Progress",
		milestone:   "1.0 Public Launch",
		estimation:  "13",
		priority:    1,
		coarse:      true,
		active:      true,
		capabilties: []string{"Quick Entry"},
	})
	p.requirement(reqSpec{
		handle:      "sighting-species",
		parent:      "sighting",
		category:    "Functionality",
		title:       "Pick a species from the catalogue",
		description: "Species selection offers the regionally plausible species first.",
		dod:         "Selecting a species from search results fills the sighting and closes the picker.",
		status:      "Done",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 12",
		estimation:  "5",
		priority:    1,
		active:      true,
		assignees:   []string{"cy"},
		capabilties: []string{"Quick Entry", "Species Search"},
	})
	p.requirement(reqSpec{
		handle:      "sighting-location",
		parent:      "sighting",
		category:    "Functionality",
		title:       "Capture the location automatically",
		description: "The device location is attached to the sighting, with a manual override.",
		dod:         "A sighting recorded with GPS available carries coordinates accurate to 10 m.",
		status:      "In Progress",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		estimation:  "5",
		priority:    2,
		active:      true,
		assignees:   []string{"cy"},
		capabilties: []string{"Quick Entry"},
	})
	p.requirement(reqSpec{
		handle:      "sighting-count",
		parent:      "sighting",
		category:    "Functionality",
		title:       "Record how many individuals were seen",
		description: "A count, with a flag for 'estimated' when the flock is too large to count.",
		status:      "New",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 14",
		estimation:  "3",
		priority:    3,
	})
	p.requirement(reqSpec{
		handle:      "sighting-photo",
		parent:      "sighting",
		category:    "Functionality",
		title:       "Attach photos to a sighting",
		description: "Up to eight photos per sighting, taken in-app or picked from the library.",
		status:      "In Review",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		estimation:  "8",
		priority:    2,
		active:      true,
		assignees:   []string{"cy"},
		capabilties: []string{"Photo Attachment"},
	})

	p.requirement(reqSpec{
		handle:      "offline",
		category:    "Functionality",
		title:       "Work with no connectivity",
		description: "Every logging feature works with the radio off; nothing is lost.",
		dod:         "A sighting recorded in airplane mode appears on the server after reconnecting.",
		status:      "In Progress",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		estimation:  "13",
		priority:    1,
		active:      true,
		assignees:   []string{"cy"},
		capabilties: []string{"Offline Sync"},
	})
	p.requirement(reqSpec{
		handle:      "sync-conflict",
		parent:      "offline",
		category:    "Functionality",
		title:       "Resolve conflicting edits after a long offline stretch",
		description: "Two devices editing the same sighting must not silently lose one of the edits.",
		status:      "New",
		milestone:   "1.0 Public Launch",
		estimation:  "8",
		priority:    2,
		coarse:      true,
		capabilties: []string{"Offline Sync"},
	})
	p.requirement(reqSpec{
		handle:      "species-search",
		category:    "Functionality",
		title:       "Search the species catalogue",
		description: "Search by common name, scientific name, or a partial match of either.",
		dod:         "Searching 'buz' returns Common Buzzard within 200 ms on a cold cache.",
		status:      "Done",
		milestone:   "0.1 Field Trial",
		sprint:      "Sprint 12",
		estimation:  "8",
		priority:    1,
		active:      true,
		capabilties: []string{"Species Search"},
	})
	p.requirement(reqSpec{
		handle:      "life-list",
		category:    "Functionality",
		title:       "Publish a life list",
		description: "A birder can publish their life list under a stable public link.",
		status:      "New",
		milestone:   "1.1 Sharing",
		estimation:  "13",
		priority:    4,
		coarse:      true,
		capabilties: []string{"Public Life List"},
	})

	p.requirement(reqSpec{
		handle:      "arch-local-store",
		category:    "Architecture",
		title:       "Local store is the single source of truth on device",
		description: "The UI reads only from the local store; sync is a background reconciliation, never a blocking fetch.",
		status:      "Done",
		milestone:   "0.1 Field Trial",
		sprint:      "Sprint 12",
		estimation:  "8",
		priority:    1,
		active:      true,
		capabilties: []string{"Offline Sync"},
	})
	p.requirement(reqSpec{
		category:    "Architecture",
		title:       "Species catalogue ships with the app bundle",
		description: "The regional catalogue is bundled so a first-run app is useful with no connection.",
		status:      "In Review",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		estimation:  "5",
		priority:    2,
		active:      true,
		capabilties: []string{"Species Search"},
	})

	p.requirement(reqSpec{
		handle:      "sec-photo-strip",
		category:    "Security",
		title:       "Strip precise coordinates from shared photos",
		description: "Photos published to a shared life list must not leak nest locations through EXIF.",
		status:      "New",
		milestone:   "1.1 Sharing",
		estimation:  "5",
		priority:    1,
		capabilties: []string{"Public Life List", "Photo Attachment"},
	})
	p.requirement(reqSpec{
		handle:      "sec-sensitive-species",
		category:    "Security",
		title:       "Blur locations of sensitive species",
		description: "Sightings of species on the sensitive list are published at reduced location precision.",
		dod:         "A published sighting of a sensitive species resolves to no better than a 10 km square.",
		status:      "New",
		milestone:   "1.1 Sharing",
		estimation:  "8",
		priority:    1,
	})

	p.requirement(reqSpec{
		handle:      "ux-one-handed",
		category:    "UX",
		title:       "Logging is usable one-handed with gloves on",
		description: "Primary targets are at least 48 dp and reachable with a thumb.",
		status:      "In Progress",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		estimation:  "5",
		priority:    2,
		active:      true,
		assignees:   []string{"cy"},
		capabilties: []string{"Quick Entry"},
	})
	p.requirement(reqSpec{
		category:    "UX",
		title:       "Readable in direct sunlight",
		description: "A high-contrast field theme that survives a bright morning on open water.",
		status:      "New",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 14",
		estimation:  "3",
		priority:    3,
	})

	// Tasks: work that carries no product statement of its own.
	p.requirement(reqSpec{
		kind:       model.RequirementKindTask,
		category:   "Tooling",
		title:      "Set up device farm runs for the nightly suite",
		status:     "In Progress",
		milestone:  "1.0 Public Launch",
		sprint:     "Sprint 13",
		estimation: "5",
		priority:   2,
		active:     true,
		assignees:  []string{"cy"},
	})
	p.requirement(reqSpec{
		kind:       model.RequirementKindTask,
		category:   "Maintenance",
		title:      "Upgrade the map SDK to the current major version",
		status:     "New",
		milestone:  "1.0 Public Launch",
		sprint:     "Sprint 14",
		estimation: "3",
		priority:   3,
	})
	p.requirement(reqSpec{
		kind:       model.RequirementKindTask,
		category:   "Maintenance",
		title:      "Retire the legacy sighting import script",
		status:     "Done",
		milestone:  "0.1 Field Trial",
		sprint:     "Sprint 12",
		estimation: "2",
		priority:   4,
		active:     true,
	})
	p.requirement(reqSpec{
		handle:      "spike-conflict",
		kind:        model.RequirementKindTask,
		category:    "Spike",
		title:       "Spike: evaluate CRDTs for sighting reconciliation",
		description: "Timebox two days. Compare a CRDT against last-write-wins with a manual merge screen.",
		status:      "New",
		milestone:   "1.0 Public Launch",
		estimation:  "3",
		priority:    3,
		coarse:      true,
	})
}

func (s *seeder) aviaryDocuments(p *productSeed) {
	p.document(documentSpec{
		title:     "Sync protocol",
		format:    model.DocumentFormatMarkdown,
		state:     model.DocumentStateCurrent,
		component: "Sync & Storage",
		milestone: "1.0 Public Launch",
		revisions: []string{
			"# Sync protocol\n\nDevices push queued sightings to `POST /v1/sync` in batches of at most 50.\n\n" +
				"## Batches\n\n- Every sighting carries a client-generated UUID.\n- The server answers with the UUIDs it stored.\n",
			"# Sync protocol\n\nDevices push queued sightings to `POST /v1/sync` in batches of at most 50.\n\n" +
				"## Batches\n\n- Every sighting carries a client-generated UUID.\n- The server answers with the UUIDs it stored.\n" +
				"- Replaying a batch is harmless: a UUID the server knows is acknowledged, not stored twice.\n\n" +
				"## Backoff\n\nAfter a failed push the device waits 2, 4, 8 … up to 300 seconds.\n",
		},
		linkReqs:  []string{"offline"},
		linkCons:  []string{"Offline-first, no blocking network calls in the logging path"},
		linkTests: []string{"tc-queue-idempotent", "tc-sync-reconnect"},
	})
	p.document(documentSpec{
		title:      "Sighting record schema",
		format:     model.DocumentFormatJSON,
		state:      model.DocumentStateCurrent,
		capability: "Offline Sync",
		milestone:  "1.0 Public Launch",
		revisions: []string{`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Sighting",
  "type": "object",
  "required": ["id", "species", "observed_at"],
  "properties": {
    "id": {"type": "string", "format": "uuid"},
    "species": {"type": "string"},
    "count": {"type": "integer", "minimum": 1},
    "observed_at": {"type": "string", "format": "date-time"}
  }
}
`},
		linkReqs: []string{"arch-local-store"},
	})
	p.document(documentSpec{
		title:     "Conflict resolution",
		format:    model.DocumentFormatMarkdown,
		state:     model.DocumentStateDraft,
		component: "Sync & Storage",
		revisions: []string{"# Conflict resolution\n\nOpen question: last write wins per field, or a merge screen?\n"},
		linkReqs:  []string{"sync-conflict"},
	})
}

func (s *seeder) aviaryConstraints(p *productSeed) {
	p.constraint(constraintSpec{
		category:    "Architecture",
		title:       "Offline-first, no blocking network calls in the logging path",
		description: "No screen in the logging flow may await a network response to complete.",
		compliance:  model.ComplianceMet,
		milestone:   "1.0 Public Launch",
		priority:    1,
		linkCaps:    []string{"Quick Entry", "Offline Sync"},
	})
	p.constraint(constraintSpec{
		category:    "Architecture",
		title:       "Bundle size stays under 60 MB",
		description: "The bundled species catalogue must not push the download past the cellular install limit.",
		compliance:  model.ComplianceViolated,
		milestone:   "1.0 Public Launch",
		priority:    2,
		linkCaps:    []string{"Species Search"},
	})
	p.constraint(constraintSpec{
		category:    "Security",
		title:       "Observation data is encrypted at rest on device",
		description: "A lost phone must not expose a birder's unpublished observations.",
		compliance:  model.ComplianceMet,
		milestone:   "1.0 Public Launch",
		priority:    1,
		linkCaps:    []string{"Offline Sync"},
	})
	p.constraint(constraintSpec{
		category:    "Security",
		title:       "No third-party analytics in the logging path",
		description: "Location traces must not leave the device outside of an explicit publish action.",
		compliance:  model.ComplianceNotAssessed,
		milestone:   "1.1 Sharing",
		priority:    2,
		linkCaps:    []string{"Public Life List"},
	})
	p.constraint(constraintSpec{
		category:    "UX",
		title:       "Meets WCAG 2.2 AA contrast in both themes",
		description: "Including the high-contrast field theme, which is the one that actually gets used outdoors.",
		compliance:  model.ComplianceNotAssessed,
		milestone:   "1.0 Public Launch",
		priority:    2,
	})
}

func (s *seeder) aviaryDefects(p *productSeed) {
	p.defect(defectSpec{
		handle:      "gps-drift",
		title:       "Location snaps to the previous sighting after a long GPS lock",
		description: "Recording two sightings within a minute reuses the first fix, so both land on the same coordinates.",
		criticality: model.DefectCriticalityHigh,
		status:      "In Progress",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		environment: "Android 14",
		estimation:  "5",
		priority:    1,
		active:      true,
		assignees:   []string{"cy"},
		linkReqs:    []string{"sighting-location"},
		linkCaps:    []string{"Quick Entry"},
	})
	p.defect(defectSpec{
		handle:      "photo-orientation",
		title:       "Attached photos are rotated 90° on upload",
		description: "EXIF orientation is dropped when the photo is re-encoded for upload.",
		criticality: model.DefectCriticalityMedium,
		status:      "In Review",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		environment: "iOS 17",
		estimation:  "3",
		priority:    2,
		active:      true,
		assignees:   []string{"cy"},
		linkReqs:    []string{"sighting-photo"},
		linkCaps:    []string{"Photo Attachment"},
	})
	p.defect(defectSpec{
		handle:      "sync-duplicates",
		title:       "Reconnecting after 24h offline duplicates every queued sighting",
		description: "The sync queue is replayed without checking the server-side idempotency key.",
		criticality: model.DefectCriticalityCritical,
		status:      "In Progress",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 13",
		estimation:  "8",
		priority:    1,
		active:      true,
		assignees:   []string{"cy"},
		linkReqs:    []string{"offline"},
		linkCaps:    []string{"Offline Sync"},
		linkComps:   []string{"Sync & Storage"},
	})
	p.defect(defectSpec{
		handle:      "search-diacritics",
		title:       "Species search ignores diacritics inconsistently",
		description: "'Grünfink' matches, 'Grunfink' does not, but 'Gruenfink' does.",
		criticality: model.DefectCriticalityLow,
		status:      "New",
		milestone:   "1.0 Public Launch",
		estimation:  "2",
		priority:    3,
		linkReqs:    []string{"species-search"},
		linkCaps:    []string{"Species Search"},
	})
	p.defect(defectSpec{
		handle:      "search-slow-cold",
		title:       "First species search after launch takes over two seconds",
		description: "The catalogue index is built lazily on the first query rather than at startup.",
		criticality: model.DefectCriticalityMedium,
		status:      "New",
		milestone:   "1.0 Public Launch",
		sprint:      "Sprint 14",
		environment: "Android 14",
		estimation:  "3",
		priority:    2,
		linkReqs:    []string{"species-search"},
	})
	p.defect(defectSpec{
		handle:      "count-keyboard",
		title:       "Count field opens the alphabetic keyboard",
		description: "Numeric input type is not set, so the field opens the wrong keyboard on both platforms.",
		criticality: model.DefectCriticalityLow,
		status:      "New",
		estimation:  "1",
		priority:    4,
		linkReqs:    []string{"sighting-count"},
	})
	p.defect(defectSpec{
		handle:      "contrast-map",
		title:       "Map overlay labels fail contrast in the field theme",
		description: "White labels on the light satellite basemap are unreadable in sunlight.",
		criticality: model.DefectCriticalityMedium,
		status:      "New",
		milestone:   "1.0 Public Launch",
		environment: "Web (Chrome)",
		estimation:  "2",
		priority:    3,
		linkReqs:    []string{"ux-one-handed"},
	})
	// Fixed during the field trial. Closed further down, once the campaign
	// result it broke exists — closing a defect pushes its failed results to
	// re-test, and that transition is part of what the fixture demonstrates.
	p.defect(defectSpec{
		handle:      "trial-crash",
		title:       "App crashes when a sighting is saved with no species selected",
		description: "Found during the field trial; the validation guard was missing on the save path.",
		criticality: model.DefectCriticalityCritical,
		status:      "In Review",
		milestone:   "0.1 Field Trial",
		sprint:      "Sprint 12",
		environment: "iOS 17",
		estimation:  "2",
		priority:    1,
		active:      true,
		linkReqs:    []string{"sighting-species"},
		linkComps:   []string{"Field Logging"},
	})

	// Aviary carries the bulk data: it is the product with enough statuses,
	// sprints and environments for every facet and sort key on the defects list
	// to have something to bite on. 60 filler rows put it over the 50-row page
	// size, so pagination is exercised on a fresh dev database.
	p.fillerDefects(60,
		[]string{"New", "In Progress", "In Review", "Done"},
		[]string{"iOS 17", "Android 14", "Web (Chrome)"},
		[]string{"Sprint 12", "Sprint 13", "Sprint 14"})
}

func (s *seeder) aviaryTestCases(p *productSeed) {
	p.testCase(testCaseSpec{
		handle:     "tc-species-search-unit",
		title:      "Species index matches partial common names",
		component:  "Species Catalogue",
		capability: "Species Search",
		level:      "Unit",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestSpeciesIndex_PartialCommonName",
		reqs:       []string{"species-search"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-species-search-diacritics",
		title:      "Species index folds diacritics both ways",
		component:  "Species Catalogue",
		capability: "Species Search",
		level:      "Unit",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestSpeciesIndex_DiacriticFolding",
		reqs:       []string{"species-search"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-store-roundtrip",
		title:      "Local store round-trips a sighting with photos",
		component:  "Sync & Storage",
		capability: "Offline Sync",
		level:      "Unit",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestLocalStore_SightingRoundTrip",
		reqs:       []string{"arch-local-store"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-queue-idempotent",
		title:      "Sync queue replay is idempotent",
		component:  "Sync & Storage",
		capability: "Offline Sync",
		level:      "Unit",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestSyncQueue_ReplayIdempotent",
		reqs:       []string{"offline"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-exif-strip",
		title:      "EXIF location is stripped before publishing",
		component:  "Sharing",
		capability: "Public Life List",
		level:      "Unit",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestPublish_StripsExifLocation",
		reqs:       []string{"sec-photo-strip"},
	})

	p.testCase(testCaseSpec{
		handle:      "tc-sync-reconnect",
		title:       "Queued sightings reach the server after reconnecting",
		description: "Record three sightings in airplane mode, restore connectivity, verify all three arrive exactly once.",
		component:   "Sync & Storage",
		capability:  "Offline Sync",
		level:       "Integration",
		mode:        model.ExecutionModeAutomatic,
		identifier:  "TestSync_ReconnectDeliversQueue",
		envs:        []string{"iOS 17", "Android 14"},
		dodReqs:     []string{"offline"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-photo-upload",
		title:      "Photo upload preserves orientation",
		component:  "Field Logging",
		capability: "Photo Attachment",
		level:      "Integration",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestPhotoUpload_PreservesOrientation",
		envs:       []string{"iOS 17", "Android 14"},
		reqs:       []string{"sighting-photo"},
	})
	p.testCase(testCaseSpec{
		handle:      "tc-catalogue-bundled",
		title:       "First run works with no network",
		description: "Install fresh, keep the radio off, confirm the species catalogue is searchable.",
		component:   "Species Catalogue",
		capability:  "Species Search",
		level:       "Integration",
		mode:        model.ExecutionModeManual,
		envs:        []string{"iOS 17", "Android 14"},
		reqs:        []string{"species-search"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-location-accuracy",
		title:      "Recorded coordinates match the device fix",
		component:  "Field Logging",
		capability: "Quick Entry",
		level:      "Integration",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestSighting_LocationMatchesFix",
		envs:       []string{"Android 14"},
		dodReqs:    []string{"sighting-location"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-species-picker",
		title:      "Species picker fills the sighting and closes",
		component:  "Field Logging",
		capability: "Quick Entry",
		level:      "Integration",
		mode:       model.ExecutionModeAutomatic,
		identifier: "TestSpeciesPicker_SelectionClosesSheet",
		dodReqs:    []string{"sighting-species"},
	})

	p.testCase(testCaseSpec{
		handle:      "tc-e2e-quick-entry",
		title:       "Log a sighting in under five seconds",
		description: "Stopwatch from app launch to saved sighting, one-handed, gloves on.",
		component:   "Field Logging",
		capability:  "Quick Entry",
		level:       "End-to-End",
		mode:        model.ExecutionModeManual,
		envs:        []string{"iOS 17", "Android 14"},
		reqs:        []string{"sighting", "ux-one-handed"},
	})
	p.testCase(testCaseSpec{
		handle:      "tc-e2e-offline-day",
		title:       "A full day offline, then reconnect",
		description: "Twenty sightings across eight hours in airplane mode, then reconnect and reconcile.",
		component:   "Sync & Storage",
		capability:  "Offline Sync",
		level:       "End-to-End",
		mode:        model.ExecutionModeManual,
		envs:        []string{"iOS 17", "Android 14"},
		reqs:        []string{"offline"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-e2e-no-species",
		title:      "Saving without a species is refused, not crashed",
		component:  "Field Logging",
		capability: "Quick Entry",
		level:      "End-to-End",
		mode:       model.ExecutionModeManual,
		envs:       []string{"iOS 17"},
		reqs:       []string{"sighting-species"},
	})
	p.testCase(testCaseSpec{
		handle:      "tc-e2e-sunlight",
		title:       "Field theme is readable in direct sunlight",
		description: "Run outdoors on a clear morning. Judgement call — record what was unreadable.",
		component:   "Field Logging",
		level:       "End-to-End",
		mode:        model.ExecutionModeManual,
		envs:        []string{"iOS 17", "Android 14"},
		reqs:        []string{"ux-one-handed"},
	})
	p.testCase(testCaseSpec{
		handle:     "tc-e2e-publish",
		title:      "Publish a life list and open the public link",
		component:  "Sharing",
		capability: "Public Life List",
		level:      "End-to-End",
		mode:       model.ExecutionModeManual,
		envs:       []string{"Web (Chrome)"},
		reqs:       []string{"life-list"},
	})
}

func (s *seeder) aviaryCampaigns(p *productSeed) {
	// Only one campaign per product may be active, so the field-trial campaign
	// is run to completion and closed before the current one is activated.
	p.campaign(
		"Release 0.1 Regression",
		"Regression pass before handing the field trial build to the ornithological society.",
		day(-24), day(-16),
		[]string{
			"tc-species-search-unit", "tc-species-search-diacritics", "tc-store-roundtrip",
			"tc-species-picker", "tc-catalogue-bundled", "tc-e2e-quick-entry", "tc-e2e-no-species",
		},
		[]verdict{
			{testCase: "tc-species-search-unit", status: model.CampaignResultStatusPassed},
			{testCase: "tc-species-search-diacritics", status: model.CampaignResultStatusFailed, defect: "search-diacritics"},
			{testCase: "tc-store-roundtrip", status: model.CampaignResultStatusPassed},
			{testCase: "tc-species-picker", status: model.CampaignResultStatusPassed},
			{testCase: "tc-catalogue-bundled", status: model.CampaignResultStatusPassed},
			{testCase: "tc-e2e-quick-entry", status: model.CampaignResultStatusPassed},
			{testCase: "tc-e2e-no-species", status: model.CampaignResultStatusFailed, defect: "trial-crash"},
		},
		"bo", true,
	)

	p.campaign(
		"Sprint 13 Verification",
		"Verifying the offline sync and photo work landing this sprint. Still in progress.",
		day(-7), day(7),
		[]string{
			"tc-queue-idempotent", "tc-sync-reconnect", "tc-photo-upload", "tc-location-accuracy",
			"tc-exif-strip", "tc-e2e-offline-day", "tc-e2e-sunlight",
		},
		[]verdict{
			{testCase: "tc-queue-idempotent", status: model.CampaignResultStatusPassed},
			{testCase: "tc-sync-reconnect", status: model.CampaignResultStatusFailed, defect: "sync-duplicates"},
			{testCase: "tc-photo-upload", status: model.CampaignResultStatusFailed, defect: "photo-orientation"},
			{testCase: "tc-location-accuracy", status: model.CampaignResultStatusFailed, defect: "gps-drift"},
			{testCase: "tc-e2e-sunlight", status: model.CampaignResultStatusBlocked},
			// tc-exif-strip and tc-e2e-offline-day stay pending, so the matrix
			// has undecided rows and the campaign reads as genuinely in flight.
		},
		"bo", false,
	)

	// Closing a defect moves the results it failed into re-test, which is the
	// flow FR-123 describes. Doing it here rather than at defect creation time
	// means there are results for it to act on.
	//
	// The photo defect belongs to the campaign that is still running, so its
	// result flips to re-test and the matrix shows a row waiting to be retried.
	// The field-trial defect belongs to the campaign that has already closed:
	// that result stays failed, because a closed campaign records the verdict
	// it was closed with and a later fix must not rewrite it.
	p.setDefectStatus("photo-orientation", "Done")
	p.setDefectStatus("trial-crash", "Done")
}

func (s *seeder) aviaryDiscussion(p *productSeed) {
	// Commenting on a requirement takes the developer role, so these are the
	// admin and cy. Defects and test cases are open to testers, which is where
	// bo's voice appears.
	p.comment("cy", model.OwnerTypeRequirement, "offline",
		"The queue replay is the risky part here. I'd rather split the reconciliation out into its own item than let this one grow.")
	p.comment("admin", model.OwnerTypeRequirement, "offline",
		"Agreed — the society's testers were offline for most of a weekend, so this needs to hold up over days, not hours.")
	p.comment("cy", model.OwnerTypeRequirement, "sighting-location",
		"Battery saver is what breaks this: the OS stops refreshing the fix and we happily reuse the stale one.")
	p.comment("cy", model.OwnerTypeRequirement, "sec-sensitive-species",
		"We need the sensitive species list from the society before this can be estimated properly.")

	p.comment("bo", model.OwnerTypeDefect, "sync-duplicates",
		"Reproduced on both platforms: 20 queued sightings came back as 40. The server accepts the same payload twice without complaint.")
	p.comment("cy", model.OwnerTypeDefect, "sync-duplicates",
		"The idempotency key is generated at send time instead of at record time, so a retry gets a fresh key. Fixing that side.")
	p.comment("bo", model.OwnerTypeDefect, "gps-drift",
		"Only reproduces when the two sightings are under 60 seconds apart. Over that, the fix refreshes correctly.")
	p.comment("bo", model.OwnerTypeDefect, "trial-crash",
		"Verified fixed on the trial build — the save button is now disabled until a species is picked.")
}
