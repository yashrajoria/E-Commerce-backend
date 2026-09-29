# Agent Service API

Endpoints:

- **POST** /agent/query — Run an analytics prompt through the agent pipeline
- **GET** /agent/session/{session_id} — Inspect in-memory session history
- **DELETE** /agent/session/{session_id} — Clear an in-memory agent session
- **GET** /tools — List available agent tools