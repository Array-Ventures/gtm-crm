-- up

ALTER TABLE organizations ADD COLUMN github_url TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS ux_organizations_github_url
    ON organizations(github_url) WHERE archived = 0 AND github_url IS NOT NULL;

ALTER TABLE people ADD COLUMN github_url TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS ux_people_github_url
    ON people(github_url) WHERE archived = 0 AND github_url IS NOT NULL;

-- down

DROP INDEX IF EXISTS ux_people_github_url;
ALTER TABLE people DROP COLUMN github_url;
DROP INDEX IF EXISTS ux_organizations_github_url;
ALTER TABLE organizations DROP COLUMN github_url;
