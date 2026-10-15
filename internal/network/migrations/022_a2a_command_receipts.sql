SET search_path=undergrid,pg_catalog;

-- A2A tasks are transport command receipts, never collaboration assignments.
-- Private command bodies, transcripts, bearer/upload tokens and result bodies
-- deliberately have no storage column in this table.
CREATE TABLE a2a_command_receipts (
 id uuid PRIMARY KEY,
 principal_id uuid NOT NULL REFERENCES principals(id),
 agent_id uuid NOT NULL REFERENCES agents(id),
 context_id uuid NOT NULL,
 message_id text NOT NULL CHECK(length(message_id) BETWEEN 1 AND 128),
 operation text NOT NULL CHECK(length(operation) BETWEEN 1 AND 120),
 command_key text NOT NULL CHECK(length(command_key) BETWEEN 1 AND 128),
 request_hash text NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
 result_hash text NOT NULL CHECK(result_hash ~ '^[0-9a-f]{64}$'),
 sequence bigint NOT NULL CHECK(sequence>0),
 command_status integer NOT NULL CHECK(command_status BETWEEN 200 AND 299),
 resource_path text NOT NULL CHECK(length(resource_path) <= 256 AND (resource_path='' OR resource_path LIKE '/v1/%')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(agent_id,sequence),
 UNIQUE(agent_id,message_id),
 UNIQUE(agent_id,operation,command_key)
);
CREATE INDEX a2a_receipts_actor_page ON a2a_command_receipts(agent_id,sequence);
CREATE INDEX a2a_receipts_actor_context ON a2a_command_receipts(agent_id,context_id);
ALTER TABLE a2a_command_receipts ENABLE ROW LEVEL SECURITY;
CREATE POLICY a2a_receipts_actor ON a2a_command_receipts
 USING(principal_id=actor_principal() AND agent_id=actor_agent())
 WITH CHECK(principal_id=actor_principal() AND agent_id=actor_agent());
GRANT SELECT,INSERT ON a2a_command_receipts TO undergrid_app;
