# Aviary Specification

A bird watching app.

Works offline.

<!-- Exported from Nexmap. Change the items in Nexmap rather than here; identifiers such as FR-12 name them in the CLI and REST API. -->

## Product Structure

### Components

#### Sync engine

Keeps devices in step.

* **Offline queue**: Queues writes.

* **Replay**

  Replays the queue.

  In order.

### Capabilities Without a Component

* **Sign-in**

## Constraints

### Security

* **SC-1**: All traffic uses TLS

* **SC-2**: Tokens are hashed

  Capabilities: Sign-in.

## Requirements

### Functionality

* **FR-1**: Record a sighting

  A user records
  what they saw.

  Definition of done:

  - saved offline
  - synced later

  Capabilities: Offline queue, Replay.

  * **FR-4**: Attach a photo
  * **FR-5**: Attach a location

* **FR-2**: List sightings

### Security

* **SR-3**: Sightings are private

  Refines FR-1.
