-- Reference data this repository owns rather than ingests.
--
-- Every id below is a LITERAL CONSTANT. Not gen_random_uuid(), not a sequence, not derived at
-- migration time. A seed that generates its ids gives local, CI and the deployed host different ids
-- for the same person, and every fixture or assertion naming a Driver breaks the moment it crosses
-- an environment. See ADR-0003.
--
-- Circuit coordinates are seeded rather than geocoded: deterministic, testable, one fewer network
-- dependency, and "Monza" geocodes to a town as readily as to a circuit.
--
-- circuit_key is the upstream's own circuit identifier. It is here so ingest can join a Session to
-- a Circuit; it is not this table's identity.
--
-- Note circuit_key 12: upstream files the 2026 "Bahrain Grand Prix in Malaysia" under location
-- 'Kuala Lumpur' with country 'Bahrain'. The country is upstream's, kept as-is; the coordinates are
-- Sepang's, because that is where the cars are.
INSERT INTO f1.circuits (id, circuit_key, short_name, location, country_name, latitude, longitude) VALUES
    ('d5ffead2-0555-4abc-b5f0-734ccd124d13',   2, 'Silverstone'       , 'Silverstone' , 'United Kingdom'      ,  52.0786,   -1.0169),
    ('895a456c-ad7f-4846-b9ed-461e8184ca63',   4, 'Hungaroring'       , 'Budapest'    , 'Hungary'             ,  47.5789,   19.2486),
    ('518f47a5-985c-4482-8a85-21704f0d50d2',   6, 'Imola'             , 'Imola'       , 'Italy'               ,  44.3439,   11.7167),
    ('65e471e0-d6c9-4003-ac43-16dfc8663eb4',   7, 'Spa-Francorchamps' , 'Spa-Francorchamps', 'Belgium'        ,  50.4372,    5.9714),
    ('5f6107b0-18cb-4251-8686-d7035569bbf5',   9, 'Austin'            , 'Austin'      , 'United States'       ,  30.1328,  -97.6411),
    ('94c8f0c4-407d-4e54-98e9-7aa15744d89e',  10, 'Melbourne'         , 'Melbourne'   , 'Australia'           , -37.8497,  144.9680),
    ('9e4d3acc-6194-46ac-9d1c-457c6fc9de06',  12, 'Kuala Lumpur'      , 'Kuala Lumpur', 'Bahrain'             ,   2.7608,  101.7382),
    ('b1bf336b-9c7f-4c0b-993e-1778322ba2b2',  14, 'Interlagos'        , 'São Paulo'   , 'Brazil'              , -23.7036,  -46.6997),
    ('b515d847-c0af-4a2a-8050-0593794220c2',  15, 'Catalunya'         , 'Barcelona'   , 'Spain'               ,  41.5700,    2.2611),
    ('fc423b06-f4bb-4b70-8fe0-c65212ccef7f',  19, 'Spielberg'         , 'Spielberg'   , 'Austria'             ,  47.2197,   14.7647),
    ('61ed99bd-ccf6-44cf-a35c-6f6f6c66b613',  22, 'Monte Carlo'       , 'Monaco'      , 'Monaco'              ,  43.7347,    7.4206),
    ('01eb57ab-c958-4053-bacf-308b9cd4aac5',  23, 'Montreal'          , 'Montréal'    , 'Canada'              ,  45.5000,  -73.5228),
    ('d895bd57-b3c6-4f2f-b2a4-b5a013592277',  39, 'Monza'             , 'Monza'       , 'Italy'               ,  45.6156,    9.2811),
    ('de624e00-8b54-4989-9a6a-9ce300dc4615',  46, 'Suzuka'            , 'Suzuka'      , 'Japan'               ,  34.8431,  136.5411),
    ('62f379ab-818d-452e-9400-ecf18c823717',  49, 'Shanghai'          , 'Shanghai'    , 'China'               ,  31.3389,  121.2200),
    ('58360c78-e47c-4ac0-a56a-df3572b1b8d7',  55, 'Zandvoort'         , 'Zandvoort'   , 'Netherlands'         ,  52.3888,    4.5409),
    ('42ab6dbf-cb19-4dcd-8483-0a44a3ac9b56',  61, 'Singapore'         , 'Marina Bay'  , 'Singapore'           ,   1.2914,  103.8640),
    ('4a143695-35ba-4998-aa09-e79dad4ce7cc',  63, 'Sakhir'            , 'Sakhir'      , 'Bahrain'             ,  26.0325,   50.5106),
    ('d9e37964-f16d-42e6-88a9-27501c001040',  65, 'Mexico City'       , 'Mexico City' , 'Mexico'              ,  19.4042,  -99.0907),
    ('66520164-1b2a-4139-a3d6-7c15e722b16c',  70, 'Yas Marina Circuit', 'Yas Island'  , 'United Arab Emirates',  24.4672,   54.6031),
    ('15b56812-f307-43fd-8994-6bb37464b1fb', 144, 'Baku'              , 'Baku'        , 'Azerbaijan'          ,  40.3725,   49.8533),
    ('274e6e94-6a45-4285-a0ab-db4c056d2de1', 149, 'Jeddah'            , 'Jeddah'      , 'Saudi Arabia'        ,  21.6319,   39.1044),
    ('6e50da2c-f030-4fc8-be72-23e943f90647', 150, 'Lusail'            , 'Lusail'      , 'Qatar'               ,  25.4900,   51.4542),
    ('42deb79d-413b-4082-a4c0-d404fb0d6c02', 151, 'Miami'             , 'Miami'       , 'United States'       ,  25.9581,  -80.2389),
    ('bf4e6217-d6e8-43e2-bcb3-508cd7390177', 152, 'Las Vegas'         , 'Las Vegas'   , 'United States'       ,  36.1147, -115.1728),
    ('613322aa-e076-4f44-a331-7c059cc3e680', 153, 'Madring'           , 'Madrid'      , 'Spain'               ,  40.4653,   -3.6167);

-- Every Driver appearing in a Race Session across 2023-2026, taken from the upstream driver roster
-- for all 96 Race Sessions in that window.
--
-- full_name is the upstream display string, and the only thing ingest resolves on. short_name is
-- the acronym, carried so either form can be shown without a second lookup.
--
-- Why identity has to originate here, visible in this list: upstream's driver_number is a
-- per-season assignment, and number 1 belongs to the reigning champion rather than to a person.
-- Across this corpus 1 covers both VERSTAPPEN and NORRIS, and 3 covers both VERSTAPPEN and
-- RICCIARDO. Keying results on the number merges two people, quietly. See ADR-0003.
INSERT INTO f1.drivers (id, full_name, short_name) VALUES
    ('35d42e7f-a4f0-46e7-8ddc-dfa63a275129', 'Alexander ALBON'   , 'ALB'),
    ('e0d25418-a566-4171-8336-59f649d0c606', 'Arvid LINDBLAD'    , 'LIN'),
    ('b1420115-4b78-4ed2-882d-5c0874b760af', 'Carlos SAINZ'      , 'SAI'),
    ('ea180ba6-7053-4a79-97ae-8d48fcd67039', 'Charles LECLERC'   , 'LEC'),
    ('9ddf4757-e218-47c4-b8b2-04f340422893', 'Daniel RICCIARDO'  , 'RIC'),
    ('4901df14-4c5d-4de6-b0ea-232914e162a3', 'Esteban OCON'      , 'OCO'),
    ('7c5c8642-ad46-4ac8-b7ef-6380a41ab4bc', 'Fernando ALONSO'   , 'ALO'),
    ('0e968f41-acaa-4e65-bd3b-ff9fd05ab0d4', 'Franco COLAPINTO'  , 'COL'),
    ('c67b1387-c98e-4277-a058-46e985519a2f', 'Gabriel BORTOLETO' , 'BOR'),
    ('a7e29d5d-a7c8-4f77-a28f-3deb534e7093', 'George RUSSELL'    , 'RUS'),
    ('3c921220-9ca4-4e07-b713-013649f722fb', 'Isack HADJAR'      , 'HAD'),
    ('4eeb5ffc-286e-43e6-997d-f7369627ed21', 'Jack DOOHAN'       , 'DOO'),
    ('2fd95c49-d2df-4138-8291-121a6215d781', 'Kevin MAGNUSSEN'   , 'MAG'),
    ('66c3e16f-7954-4ccb-a43e-f1a6a1560f19', 'Kimi ANTONELLI'    , 'ANT'),
    ('4d2af0d5-a8f9-44f9-8a15-f3948f8da39e', 'Lance STROLL'      , 'STR'),
    ('d6588e29-d18d-43a3-85cd-3cb8c1e7d877', 'Lando NORRIS'      , 'NOR'),
    ('fd114b4f-8fd0-49ed-bc8c-726f62983cee', 'Lewis HAMILTON'    , 'HAM'),
    ('3bb8208f-0e77-4605-bc90-5445de3d28cf', 'Liam LAWSON'       , 'LAW'),
    ('d0bcb5bd-b98d-4d60-9630-d08995972a6f', 'Logan SARGEANT'    , 'SAR'),
    ('7948efdf-1ca6-4274-bc5e-b392565c13f0', 'Max VERSTAPPEN'    , 'VER'),
    ('cbbba492-8570-4349-9ab9-c0b24a0b0145', 'Nico HULKENBERG'   , 'HUL'),
    ('726c628c-95ce-4121-b903-3f3f18b6eb8d', 'Nyck DE VRIES'     , 'DEV'),
    ('62127f9b-44c1-4937-aec5-b5bd61c105c5', 'Oliver BEARMAN'    , 'BEA'),
    ('705c7fb6-2bba-4073-bd7f-4a9ee6649e86', 'Oscar PIASTRI'     , 'PIA'),
    ('75f746ae-7c2b-4e05-8b14-3504758d7a26', 'Pierre GASLY'      , 'GAS'),
    ('a970ca8f-05e9-4445-bffd-59b4adb444ba', 'Sergio PEREZ'      , 'PER'),
    ('8a039c71-21a7-482c-9090-a0dc62fb7ce0', 'Valtteri BOTTAS'   , 'BOT'),
    ('c842d6ac-987f-434b-a77d-592f0c29526b', 'Yuki TSUNODA'      , 'TSU'),
    ('059ff34f-2331-4417-955c-743f5c96f24a', 'ZHOU Guanyu'       , 'ZHO');
