-- Disposable product read model. No Copy/Canonical/Journal truth is modified.
CREATE TABLE product_library_snapshots (
 binding_id uuid PRIMARY KEY REFERENCES storage_bindings(storage_binding_id),
 root_id text NOT NULL,
 journal_seq bigint NOT NULL CHECK(journal_seq>=0),
 checkpoint text NOT NULL CHECK((journal_seq=0 AND checkpoint='') OR (journal_seq>0 AND length(checkpoint)=64)),
 revision text NOT NULL CHECK(length(revision)=64),
 updated_at timestamptz NOT NULL,
 resources jsonb NOT NULL CHECK(jsonb_typeof(resources)='array' AND octet_length(resources::text)<=67108864)
);
