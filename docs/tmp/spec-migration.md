# Migrating the specification into Nexmap

Nexmap's own specification moves from `docs/spec/` into the Nexmap product on cloud.nexmap.net
(`.nexmap.json`). Nexmap draws identifier numbers from one counter per product and entity type, unique
across categories, so the legacy identifiers cannot be kept: `AC-001` and `SC-001` cannot both exist.
Every migrated item names its legacy identifier at the end of its description.

## Status

1. **Structure: done.** 7 components and 39 capabilities.
2. **Constraints: done.** All 85 constraints from `docs/spec/requirements.md`, compliance `not_assessed`,
   linked to capabilities where they concern one feature area. Constraints that apply to the whole
   code base or a whole component (AC-1 to AC-6, AC-11, AC-13, AC-14, AC-17 to AC-19, AC-22 to AC-25)
   are not linked.
3. **Backlog: done.** All 142 requirements from `docs/spec/requirements.md` as FR-1 to FR-142, in spec
   order, each linked to its capabilities; identifiers inside their texts are rewritten to the new
   ones. 130 are `Done` because the code implements them (judged from the code, not verified by test
   cases); 12 are `New`: repository integration (FR-84, FR-86, FR-87, FR-112), notifications (FR-125
   to FR-127), search (FR-128, FR-129) and bulk operations (FR-130 to FR-132). No definitions of done
   yet. Defects found on the way: DF-1, DF-2.
4. **Documents: open.** `docs/spec/design-system.md` and `docs/spec/ui-structure.md` as documents.
5. **Repository: identifiers done.** Legacy identifiers in all tracked files except
   `docs/spec/requirements.md` (marked superseded) are replaced by the Nexmap ones (144 files);
   `docs/spec/ui-structure.md` and `docs/spec/design-system.md` included, ready for step 4. Open:
   retire `docs/spec/` and this plan once step 4 is done.
6. **Test cases: open.** Register the Go tests with `nexmap test sync` and link them to requirements.

## Constraints: legacy → Nexmap

| Legacy | Nexmap | Legacy | Nexmap | Legacy | Nexmap |
|---|---|---|---|---|---|
| AC-001 | AC-1 | SC-001 | SC-26 | UC-001 | UC-60 |
| AC-002 | AC-2 | SC-002 | SC-27 | UC-002 | UC-61 |
| AC-003 | AC-3 | SC-101 | SC-28 | UC-003 | UC-62 |
| AC-004 | AC-4 | SC-102 | SC-29 | UC-004 | UC-63 |
| AC-005 | AC-5 | SC-103 | SC-30 | UC-005 | UC-64 |
| AC-006 | AC-6 | SC-104 | SC-31 | UC-006 | UC-65 |
| AC-101 | AC-7 | SC-105 | SC-32 | UC-007 | UC-66 |
| AC-102 | AC-8 | SC-106 | SC-33 | UC-008 | UC-67 |
| AC-103 | AC-9 | SC-107 | SC-34 | UC-009 | UC-68 |
| AC-104 | AC-10 | SC-108 | SC-35 | UC-101 | UC-69 |
| AC-201 | AC-11 | SC-109 | SC-36 | UC-102 | UC-70 |
| AC-211 | AC-12 | SC-110 | SC-37 | UC-103 | UC-71 |
| AC-221 | AC-13 | SC-111 | SC-38 | UC-104 | UC-72 |
| AC-222 | AC-14 | SC-201 | SC-39 | UC-104a | UC-73 |
| AC-223 | AC-15 | SC-202 | SC-40 | UC-104b | UC-74 |
| AC-224 | AC-16 | SC-203 | SC-41 | UC-104c | UC-75 |
| AC-225 | AC-17 | SC-204 | SC-42 | UC-105 | UC-76 |
| AC-401 | AC-18 | SC-205 | SC-43 | UC-106 | UC-77 |
| AC-402 | AC-19 | SC-206 | SC-44 | UC-107 | UC-78 |
| AC-411 | AC-20 | SC-301 | SC-45 | UC-201 | UC-79 |
| AC-412 | AC-21 | SC-302 | SC-46 | UC-202 | UC-80 |
| AC-421 | AC-22 | SC-303 | SC-47 | UC-203 | UC-81 |
| AC-422 | AC-23 | SC-304 | SC-48 | UC-204 | UC-82 |
| AC-423 | AC-24 | SC-305 | SC-49 | UC-205 | UC-83 |
| AC-424 | AC-25 | SC-306 | SC-50 | UC-206 | UC-84 |
| | | SC-401 | SC-51 | UC-207 | UC-85 |
| | | SC-402 | SC-52 | | |
| | | SC-403 | SC-53 | | |
| | | SC-404 | SC-54 | | |
| | | SC-405 | SC-55 | | |
| | | SC-406 | SC-56 | | |
| | | SC-407 | SC-57 | | |
| | | SC-408 | SC-58 | | |
| | | SC-409 | SC-59 | | |

## Requirements: legacy → Nexmap

| Legacy | Nexmap | Legacy | Nexmap | Legacy | Nexmap |
|---|---|---|---|---|---|
| FR-001 | FR-1 | FR-043 | FR-49 | FR-323a | FR-97 |
| FR-002 | FR-2 | FR-044 | FR-50 | FR-324 | FR-98 |
| FR-003 | FR-3 | FR-045 | FR-51 | FR-325 | FR-99 |
| FR-004 | FR-4 | FR-046 | FR-52 | FR-326 | FR-100 |
| FR-005 | FR-5 | FR-047 | FR-53 | FR-327 | FR-101 |
| FR-005a | FR-6 | FR-048 | FR-54 | FR-328 | FR-102 |
| FR-006 | FR-7 | FR-049 | FR-55 | FR-329 | FR-103 |
| FR-007 | FR-8 | FR-050 | FR-56 | FR-330 | FR-104 |
| FR-008 | FR-9 | FR-051 | FR-57 | FR-331 | FR-105 |
| FR-008a | FR-10 | FR-052 | FR-58 | FR-332 | FR-106 |
| FR-008b | FR-11 | FR-101 | FR-59 | FR-333 | FR-107 |
| FR-008c | FR-12 | FR-102 | FR-60 | FR-334 | FR-108 |
| FR-008d | FR-13 | FR-103 | FR-61 | FR-335 | FR-109 |
| FR-008e | FR-14 | FR-104 | FR-62 | FR-401 | FR-110 |
| FR-009 | FR-15 | FR-105 | FR-63 | FR-402 | FR-111 |
| FR-010 | FR-16 | FR-106 | FR-64 | FR-403 | FR-112 |
| FR-011 | FR-17 | FR-107 | FR-65 | FR-404 | FR-113 |
| FR-012 | FR-18 | FR-201 | FR-66 | FR-405 | FR-114 |
| FR-013 | FR-19 | FR-202 | FR-67 | FR-406 | FR-115 |
| FR-014 | FR-20 | FR-203 | FR-68 | FR-407 | FR-116 |
| FR-015 | FR-21 | FR-204 | FR-69 | FR-408 | FR-117 |
| FR-016 | FR-22 | FR-205 | FR-70 | FR-409 | FR-118 |
| FR-017 | FR-23 | FR-206 | FR-71 | FR-409a | FR-119 |
| FR-018 | FR-24 | FR-207 | FR-72 | FR-501 | FR-120 |
| FR-019 | FR-25 | FR-301 | FR-73 | FR-502 | FR-121 |
| FR-020 | FR-26 | FR-302 | FR-74 | FR-503 | FR-122 |
| FR-021 | FR-27 | FR-303 | FR-75 | FR-504 | FR-123 |
| FR-022 | FR-28 | FR-304 | FR-76 | FR-505 | FR-124 |
| FR-023 | FR-29 | FR-305 | FR-77 | FR-601 | FR-125 |
| FR-024 | FR-30 | FR-306 | FR-78 | FR-602 | FR-126 |
| FR-025 | FR-31 | FR-307 | FR-79 | FR-603 | FR-127 |
| FR-026 | FR-32 | FR-308 | FR-80 | FR-701 | FR-128 |
| FR-027 | FR-33 | FR-308a | FR-81 | FR-702 | FR-129 |
| FR-028 | FR-34 | FR-309 | FR-82 | FR-801 | FR-130 |
| FR-029 | FR-35 | FR-310 | FR-83 | FR-802 | FR-131 |
| FR-030 | FR-36 | FR-311 | FR-84 | FR-803 | FR-132 |
| FR-031 | FR-37 | FR-312 | FR-85 | FR-901 | FR-133 |
| FR-032 | FR-38 | FR-313 | FR-86 | FR-902 | FR-134 |
| FR-033 | FR-39 | FR-314 | FR-87 | FR-903 | FR-135 |
| FR-034 | FR-40 | FR-315 | FR-88 | FR-904 | FR-136 |
| FR-035 | FR-41 | FR-316 | FR-89 | FR-905 | FR-137 |
| FR-036 | FR-42 | FR-317 | FR-90 | FR-906 | FR-138 |
| FR-037 | FR-43 | FR-318 | FR-91 | FR-907 | FR-139 |
| FR-038 | FR-44 | FR-319 | FR-92 | FR-908 | FR-140 |
| FR-039 | FR-45 | FR-320 | FR-93 | FR-909 | FR-141 |
| FR-040 | FR-46 | FR-321 | FR-94 | FR-910 | FR-142 |
| FR-041 | FR-47 | FR-322 | FR-95 |  |  |
| FR-042 | FR-48 | FR-323 | FR-96 |  |  |
