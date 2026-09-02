-- Withdraws the seeded reference data, leaving the tables 000001 created standing.
--
-- Two different deletes, for two different reasons.
--
-- The seeded rows go by their literal ids, so a row a later migration or an operator added to
-- f1.circuits or f1.drivers survives — a bare DELETE there would take it too.
--
-- The ingested tables are cleared wholesale first, because every one of them references a seeded
-- Circuit or Driver and Postgres will not let the seed go while they do. That is not a widening of
-- scope, it is the scope: "roll back the seed" necessarily means "and everything ingested against
-- it". Those rows are re-ingestable by definition — `make ingest` rebuilds them — where the seeded
-- ids being withdrawn are the thing that cannot be regenerated (ADR-0003).
DELETE FROM f1.session_results;
DELETE FROM f1.weather_samples;
DELETE FROM f1.sessions;
DELETE FROM f1.meetings;

DELETE FROM f1.drivers WHERE id IN (
    '35d42e7f-a4f0-46e7-8ddc-dfa63a275129', 'e0d25418-a566-4171-8336-59f649d0c606',
    'b1420115-4b78-4ed2-882d-5c0874b760af', 'ea180ba6-7053-4a79-97ae-8d48fcd67039',
    '9ddf4757-e218-47c4-b8b2-04f340422893', '4901df14-4c5d-4de6-b0ea-232914e162a3',
    '7c5c8642-ad46-4ac8-b7ef-6380a41ab4bc', '0e968f41-acaa-4e65-bd3b-ff9fd05ab0d4',
    'c67b1387-c98e-4277-a058-46e985519a2f', 'a7e29d5d-a7c8-4f77-a28f-3deb534e7093',
    '3c921220-9ca4-4e07-b713-013649f722fb', '4eeb5ffc-286e-43e6-997d-f7369627ed21',
    '2fd95c49-d2df-4138-8291-121a6215d781', '66c3e16f-7954-4ccb-a43e-f1a6a1560f19',
    '4d2af0d5-a8f9-44f9-8a15-f3948f8da39e', 'd6588e29-d18d-43a3-85cd-3cb8c1e7d877',
    'fd114b4f-8fd0-49ed-bc8c-726f62983cee', '3bb8208f-0e77-4605-bc90-5445de3d28cf',
    'd0bcb5bd-b98d-4d60-9630-d08995972a6f', '7948efdf-1ca6-4274-bc5e-b392565c13f0',
    'cbbba492-8570-4349-9ab9-c0b24a0b0145', '726c628c-95ce-4121-b903-3f3f18b6eb8d',
    '62127f9b-44c1-4937-aec5-b5bd61c105c5', '705c7fb6-2bba-4073-bd7f-4a9ee6649e86',
    '75f746ae-7c2b-4e05-8b14-3504758d7a26', 'a970ca8f-05e9-4445-bffd-59b4adb444ba',
    '8a039c71-21a7-482c-9090-a0dc62fb7ce0', 'c842d6ac-987f-434b-a77d-592f0c29526b',
    '059ff34f-2331-4417-955c-743f5c96f24a'
);

DELETE FROM f1.circuits WHERE id IN (
    'd5ffead2-0555-4abc-b5f0-734ccd124d13', '895a456c-ad7f-4846-b9ed-461e8184ca63',
    '518f47a5-985c-4482-8a85-21704f0d50d2', '65e471e0-d6c9-4003-ac43-16dfc8663eb4',
    '5f6107b0-18cb-4251-8686-d7035569bbf5', '94c8f0c4-407d-4e54-98e9-7aa15744d89e',
    '9e4d3acc-6194-46ac-9d1c-457c6fc9de06', 'b1bf336b-9c7f-4c0b-993e-1778322ba2b2',
    'b515d847-c0af-4a2a-8050-0593794220c2', 'fc423b06-f4bb-4b70-8fe0-c65212ccef7f',
    '61ed99bd-ccf6-44cf-a35c-6f6f6c66b613', '01eb57ab-c958-4053-bacf-308b9cd4aac5',
    'd895bd57-b3c6-4f2f-b2a4-b5a013592277', 'de624e00-8b54-4989-9a6a-9ce300dc4615',
    '62f379ab-818d-452e-9400-ecf18c823717', '58360c78-e47c-4ac0-a56a-df3572b1b8d7',
    '42ab6dbf-cb19-4dcd-8483-0a44a3ac9b56', '4a143695-35ba-4998-aa09-e79dad4ce7cc',
    'd9e37964-f16d-42e6-88a9-27501c001040', '66520164-1b2a-4139-a3d6-7c15e722b16c',
    '15b56812-f307-43fd-8994-6bb37464b1fb', '274e6e94-6a45-4285-a0ab-db4c056d2de1',
    '6e50da2c-f030-4fc8-be72-23e943f90647', '42deb79d-413b-4082-a4c0-d404fb0d6c02',
    'bf4e6217-d6e8-43e2-bcb3-508cd7390177', '613322aa-e076-4f44-a331-7c059cc3e680'
);
