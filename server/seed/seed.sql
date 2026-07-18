-- Seed data: initial state before any event history exists.
-- Each project's last_seq stays 0 and the events table stays empty —
-- events begin once the API starts producing them in a later phase.

TRUNCATE TABLE events, comments, task_dependencies, tasks, projects CASCADE;

-- Projects -------------------------------------------------------------

INSERT INTO projects (id, name, description, metadata, last_seq, created_at, updated_at) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Website Redesign',
     'Rebuild the marketing site on the new design system.',
     '{"color": "indigo"}', 0, now() - interval '14 days', now() - interval '1 day'),
    ('22222222-2222-2222-2222-222222222222', 'Mobile App',
     'Native companion app for iOS and Android.',
     '{"color": "teal"}', 0, now() - interval '7 days', now() - interval '2 days');

-- Project 1: Website Redesign — 8 tasks across todo/in_progress/done -----

INSERT INTO tasks (id, project_id, title, status, assigned_to, configuration, created_at, updated_at) VALUES
    ('11111111-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111',
     'Build API', 'done', '{alice}',
     '{"priority": "high", "description": "REST endpoints for pages and assets.", "tags": ["backend","api"], "customFields": {}}',
     now() - interval '13 days', now() - interval '9 days'),
    ('11111111-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111',
     'Write tests', 'in_progress', '{bob}',
     '{"priority": "medium", "description": "Cover the new API endpoints.", "tags": ["backend","testing"], "customFields": {}}',
     now() - interval '9 days', now() - interval '2 days'),
    ('11111111-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111',
     'Deploy', 'todo', '{}',
     '{"priority": "high", "description": "Ship to production once tests are green.", "tags": ["ops"], "customFields": {}}',
     now() - interval '9 days', now() - interval '9 days'),
    ('11111111-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111',
     'Design homepage mockups', 'done', '{alice}',
     '{"priority": "medium", "description": "Hi-fi mockups for the new homepage.", "tags": ["design"], "customFields": {}}',
     now() - interval '14 days', now() - interval '11 days'),
    ('11111111-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111',
     'Implement homepage', 'in_progress', '{bob,alice}',
     '{"priority": "high", "description": "Build the homepage from approved mockups.", "tags": ["frontend"], "customFields": {}}',
     now() - interval '10 days', now() - interval '1 day'),
    ('11111111-0000-0000-0000-000000000006', '11111111-1111-1111-1111-111111111111',
     'Set up CI pipeline', 'todo', '{}',
     '{"priority": "low", "description": "Lint, test, and build on every PR.", "tags": ["ops"], "customFields": {}}',
     now() - interval '6 days', now() - interval '6 days'),
    ('11111111-0000-0000-0000-000000000007', '11111111-1111-1111-1111-111111111111',
     'Write documentation', 'todo', '{}',
     '{"priority": "low", "description": "Onboarding docs for the new stack.", "tags": ["docs"], "customFields": {}}',
     now() - interval '5 days', now() - interval '5 days'),
    ('11111111-0000-0000-0000-000000000008', '11111111-1111-1111-1111-111111111111',
     'QA pass', 'todo', '{}',
     '{"priority": "medium", "description": "Full regression pass before launch.", "tags": ["qa"], "customFields": {}}',
     now() - interval '3 days', now() - interval '3 days');

-- Dependency chain: Deploy depends on Build API and Write tests;
-- Write tests depends on Build API.
INSERT INTO task_dependencies (task_id, depends_on_task_id) VALUES
    ('11111111-0000-0000-0000-000000000002', '11111111-0000-0000-0000-000000000001'),
    ('11111111-0000-0000-0000-000000000003', '11111111-0000-0000-0000-000000000001'),
    ('11111111-0000-0000-0000-000000000003', '11111111-0000-0000-0000-000000000002');

-- Comments spread over 2 tasks (Build API, Write tests), authors alice/bob.
INSERT INTO comments (id, task_id, content, author, created_at) VALUES
    ('11111111-c000-0000-0000-000000000001', '11111111-0000-0000-0000-000000000001',
     'Pushed initial endpoints, ready for review.', 'alice', now() - interval '10 days'),
    ('11111111-c000-0000-0000-000000000002', '11111111-0000-0000-0000-000000000001',
     'Looks good, left a couple nits on the PR.', 'bob', now() - interval '10 days'),
    ('11111111-c000-0000-0000-000000000003', '11111111-0000-0000-0000-000000000001',
     'Addressed feedback, merging.', 'alice', now() - interval '9 days'),
    ('11111111-c000-0000-0000-000000000004', '11111111-0000-0000-0000-000000000002',
     'Coverage is at 60%, need more edge cases.', 'bob', now() - interval '3 days'),
    ('11111111-c000-0000-0000-000000000005', '11111111-0000-0000-0000-000000000002',
     'Added tests for the auth flow.', 'alice', now() - interval '2 days');

-- Project 2: Mobile App — 3 simple tasks, no dependencies ---------------

INSERT INTO tasks (id, project_id, title, status, assigned_to, configuration, created_at, updated_at) VALUES
    ('22222222-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222',
     'Set up React Native project', 'todo', '{}',
     '{"priority": "high", "description": "Scaffold the RN app with the shared design system.", "tags": ["setup"], "customFields": {}}',
     now() - interval '6 days', now() - interval '6 days'),
    ('22222222-0000-0000-0000-000000000002', '22222222-2222-2222-2222-222222222222',
     'Build login screen', 'todo', '{}',
     '{"priority": "medium", "description": "Email/password login with validation.", "tags": ["frontend"], "customFields": {}}',
     now() - interval '5 days', now() - interval '5 days'),
    ('22222222-0000-0000-0000-000000000003', '22222222-2222-2222-2222-222222222222',
     'Configure push notifications', 'todo', '{}',
     '{"priority": "low", "description": "Wire up APNs/FCM for the app.", "tags": ["infra"], "customFields": {}}',
     now() - interval '4 days', now() - interval '4 days');
