# Agent Service API

Endpoints:

- **POST** /agent/query — Run an analytics prompt through the agent pipeline
- **GET** /agent/session/{session_id} — Inspect in-memory session history
- **DELETE** /agent/session/{session_id} — Clear an in-memory agent session
- **GET** /tools — List available agent tools
- **GET** /agent/tools — List available agent tools (alias)
- **GET** /agent/mutations/{request_id} — Inspect pending mutation proposal
- **POST** /agent/mutations/{request_id}/confirm — Confirm or reject a pending mutation