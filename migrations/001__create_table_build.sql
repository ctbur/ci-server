CREATE TYPE build_status AS ENUM (
    'pending',
    'running',
    'success',
    'failure',
    'canceled'
);

CREATE TABLE builds (
    id BIGSERIAL PRIMARY KEY,

    repo_id BIGINT NOT NULL,
    number BIGINT NOT NULL,
    link VARCHAR(255) NOT NULL,
    ref VARCHAR(255) NOT NULL,
    commit_sha VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    author VARCHAR(255) NOT NULL,

    created TIMESTAMP NOT NULL,
    started TIMESTAMP,
    finished TIMESTAMP,
    status build_status NOT NULL DEFAULT 'pending',

    CONSTRAINT fk_repo
        FOREIGN KEY (repo_id)
        REFERENCES repos (id)
        ON DELETE CASCADE,

    UNIQUE (repo_id, number)
);
