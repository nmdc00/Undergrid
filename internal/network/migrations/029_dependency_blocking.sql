SET search_path=undergrid,pg_catalog;
-- Dependency impediment is orthogonal to accepted work and assignment authority.
ALTER TABLE tasks ADD COLUMN dependency_blocked boolean NOT NULL DEFAULT false;
GRANT UPDATE(dependency_blocked) ON tasks TO undergrid_app;
CREATE FUNCTION preserve_dependency_blocking() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF OLD.dependency_blocked AND NOT NEW.dependency_blocked THEN
  RAISE EXCEPTION 'dependency blocking survives the old task claim';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER preserve_dependency_blocking BEFORE UPDATE OF dependency_blocked ON tasks FOR EACH ROW EXECUTE FUNCTION preserve_dependency_blocking();
