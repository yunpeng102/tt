
CREATE TABLE IF NOT EXISTS task (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    closed_at TIMESTAMP NULL,
    task_state TEXT CHECK(task_state IN ('open', 'in_progress', 'closed', 'cancelled')) DEFAULT 'open',
    task_content TEXT NOT NULL,
    task_spoc TEXT NULL
);

-- Create trigger to automatically update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_task_timestamp 
    AFTER UPDATE ON task
BEGIN
    UPDATE task SET updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.id;
END;


-- Insert dummy data only if the table is empty
INSERT INTO task (task_content, task_spoc, task_state, closed_at)
SELECT * FROM (
    VALUES 
        ('Setup development environment for new project', 'John Doe', 'closed', DATETIME('now', '-5 days')),
        ('Review pull request #123 for authentication module', 'Jane Smith', 'in_progress', NULL),
        ('Investigate performance issues in production', 'Mike Johnson', 'open', NULL),
        ('Update documentation for API endpoints', 'Sarah Wilson', 'open', NULL),
        ('Fix bug in user registration flow', 'John Doe', 'in_progress', NULL),
        ('Deploy v2.0 to staging environment', 'Jane Smith', 'cancelled', DATETIME('now', '-1 day')),
        ('Implement new dashboard features', 'Mike Johnson', 'open', NULL),
        ('Conduct security audit', 'Sarah Wilson', 'open', NULL),
        ('Optimize database queries', 'John Doe', 'in_progress', NULL),
        ('Setup monitoring alerts', 'Jane Smith', 'closed', DATETIME('now', '-2 days'))
) 
WHERE NOT EXISTS (SELECT 1 FROM task LIMIT 1);