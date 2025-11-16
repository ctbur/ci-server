CREATE TABLE builders (
    build_id BIGSERIAL PRIMARY KEY,

    pid INT NOT NULL,
    cache_id BIGINT,

    CONSTRAINT fk_build
        FOREIGN KEY (build_id)
        REFERENCES builds (id)
        ON DELETE CASCADE
);
