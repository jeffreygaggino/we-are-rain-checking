-- Drops exactly what the up created, in reverse. Indexes go with their tables.
DROP TABLE IF EXISTS f1.session_results;
DROP TABLE IF EXISTS f1.weather_samples;
DROP TABLE IF EXISTS f1.sessions;
DROP TABLE IF EXISTS f1.meetings;
DROP TABLE IF EXISTS f1.drivers;
DROP TABLE IF EXISTS f1.circuits;

DROP SCHEMA IF EXISTS f1;
