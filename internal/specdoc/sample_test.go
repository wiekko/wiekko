package specdoc

func sampleDocument() Document {
	return Document{
		Product:     "Aviary",
		Description: "A bird watching app.\r\n\r\nWorks offline.  ",
		Components: []Component{{
			Name: "Sync engine", Description: "Keeps devices in step.",
			Capabilities: []Capability{{Name: "Offline queue", Description: "Queues writes."}, {Name: "Replay", Description: "Replays the queue.\n\nIn order."}},
		}},
		Capabilities: []Capability{{Name: "Sign-in"}},
		Documents: []DocumentEntry{
			{Identifier: "DOC-1", Title: "Sync  protocol", Format: "markdown", State: "current", Revision: 3,
				File: "DOC-1.md", Requirements: []string{"FR-1"}, Constraints: []string{"SC-1"}, TestCases: []string{"Replay keeps order"}},
			{Identifier: "DOC-2", Title: "Sighting schema", Format: "json", State: "draft", Revision: 1, File: "DOC-2.json"},
		},
		Constraints: []Section{{Category: "Security", Items: []Item{
			{Identifier: "SC-1", Title: "All traffic uses TLS"},
			{Identifier: "SC-2", Title: "Tokens are hashed", Capabilities: []string{"Sign-in"}},
		}}},
		Requirements: []Section{
			{Category: "Functionality", Items: []Item{
				{Identifier: "FR-1", Title: "Record a sighting", Description: "A user records\nwhat they saw.",
					DefinitionOfDone: "- saved offline\n- synced later", Capabilities: []string{"Offline queue", "Replay"},
					Documents: []string{"DOC-1"},
					Children:  []Item{{Identifier: "FR-4", Title: "Attach a photo"}, {Identifier: "FR-5", Title: "Attach a  location"}}},
				{Identifier: "FR-2", Title: "List sightings"},
			}},
			{Category: "Security", Items: []Item{{Identifier: "SR-3", Title: "Sightings are private", Refines: "FR-1"}}},
		},
	}
}
