-- The stage a running job has reached: a pipeline stage name or "verify".
ALTER TABLE jobs ADD COLUMN progress text;
