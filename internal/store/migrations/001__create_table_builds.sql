CREATE TYPE build_result AS ENUM (
    'success',
    'failure',
    'canceled',
    'timeout',
    'error'
);

CREATE TABLE builds (
    id BIGSERIAL PRIMARY KEY,

    repo_id BIGINT NOT NULL,
    number BIGINT NOT NULL,
    link VARCHAR(255) NOT NULL,
    ref VARCHAR(255) NOT NULL,
    commit_sha VARCHAR(40) NOT NULL,
    message TEXT NOT NULL,
    author VARCHAR(255) NOT NULL,

    created TIMESTAMP WITH TIME ZONE NOT NULL,
    started TIMESTAMP WITH TIME ZONE,
    finished TIMESTAMP WITH TIME ZONE,
    result build_result,

    CONSTRAINT fk_repo
        FOREIGN KEY (repo_id)
        REFERENCES repos (id)
        ON DELETE CASCADE,

    UNIQUE (repo_id, number)
);
