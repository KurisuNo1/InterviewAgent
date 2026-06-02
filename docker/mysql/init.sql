-- MySQL init script for InterviewAgent
-- This runs automatically on first container start

CREATE DATABASE IF NOT EXISTS interview_agent DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE interview_agent;

-- Interview sessions
CREATE TABLE IF NOT EXISTS interview_sessions (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(64) DEFAULT '',
    status VARCHAR(32) DEFAULT 'created',
    jd_text TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Interview results (evaluations + reports)
CREATE TABLE IF NOT EXISTS interview_results (
    id VARCHAR(64) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    evaluations JSON,
    overall_score DECIMAL(5,2),
    dimension_scores JSON,
    report_json JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Review plans
CREATE TABLE IF NOT EXISTS review_plans (
    id VARCHAR(64) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    plan_json JSON,
    resources_json JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Per-question answers
CREATE TABLE IF NOT EXISTS interview_answers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    question_id VARCHAR(64) NOT NULL,
    question TEXT NOT NULL,
    answer TEXT,
    score DECIMAL(5,2),
    feedback TEXT,
    question_num INT NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
