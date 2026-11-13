# Aviary Specification

Birds, offline.

<!-- Exported from Nexmap. Change the items in Nexmap rather than here; identifiers such as FR-12 name them in the CLI and REST API. -->

## Product Structure

### Components

#### accounts

#### Sync engine

Keeps devices in step.

* **Offline queue**: Queues writes.

### Capabilities Without a Component

* **Sign-in**

## Constraints

### Security

* **SC-1**: All traffic uses TLS

  Capabilities: Sign-in.

## Requirements

### Functionality

* **FR-1**: Record a sighting

  A user records what they saw.

  Definition of done:

  - saved offline
  - synced later

  Capabilities: Offline queue.

  * **FR-2**: Attach a photo

### Security

* **SR-3**: Sightings are private

  Refines FR-1.

## Tasks

### Maintenance

* **MT-4**: Upgrade SQLite

  Refines FR-1.
