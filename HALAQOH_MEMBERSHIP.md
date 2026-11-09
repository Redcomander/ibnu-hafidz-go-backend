# Teacher Halaqoh membership requests

Teachers may propose additions, replacements/removals through the roster editor,
single-student removals, or removal of their entire own roster. They cannot change
the teacher target. An admin/super_admin must approve the request before active
assignments change. Removing membership never deletes a master student record.

The restricted picker exposes student ID, name, gender, and current Halaqoh
teacher identity. Teachers can search any student using at least two characters;
results are capped at 20 per page. Without a search, only their own roster is
returned. Wildcard-only searches do not expose the directory. The form never
downloads all student pages automatically, and selected students remain visible
while changing search terms or pages.

Teachers can propose moving a student from another Halaqoh, but cannot apply the
transfer. Admin review shows the source teacher and a Setujui Perpindahan action.
The approval API requires confirm_transfers=true for such transfers. The source
ownership snapshot is rechecked during approval, and source and destination
teachers are locked together to coordinate concurrent roster changes.
Transfers deactivate the source membership and create a new destination
assignment, preserving the original teacher on historical attendance records.
Admins retain direct roster management.

Only one pending request per teacher is accepted. Requests store the previous
roster and helper so stale approvals cannot silently overwrite later changes.
Approval/rejection is single-use and transactional. A request that no longer
matches the roster must be rejected and submitted again. Legacy pending requests
without a complete snapshot may also need to be rejected and resubmitted.

The teacher sees pending/approved/rejected requests on the Halaqoh page. Admins
review previous and proposed student names through Validasi Pengajuan Guru.
The form no longer calls the master /students or /users APIs. Teacher-role
fallbacks no longer grant general student/user directory viewing; explicitly
assigned directory permissions continue to work as configured.

Deploy backend and frontend together. The existing startup migration already
creates the approval-request table. It now also adds the nullable JSON column
student_owner_snapshot to that table; it does not remove existing columns or
modify student records in a migration. The added display details are virtual
fields. No production database changes were executed during development.
