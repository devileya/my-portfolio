-- The posting's salary range as numbers, so forms that ask for an expected
-- salary can answer from the range the employer offers.
ALTER TABLE job_listings
    ADD COLUMN salary_min      BIGINT,
    ADD COLUMN salary_max      BIGINT,
    ADD COLUMN salary_currency TEXT NOT NULL DEFAULT '',
    -- month | year | hour | '' (unknown)
    ADD COLUMN salary_period   TEXT NOT NULL DEFAULT '';
