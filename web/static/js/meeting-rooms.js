// Meeting Rooms UI Module
// Manages the Meeting Rooms view for scheduling and managing agent meetings

function renderMeetingRooms() {
    const container = document.getElementById('meeting-rooms');
    if (!container) return;

    const html = `
        <div class="meeting-rooms-container">
            <div class="toolbar" role="region" aria-label="Meeting room controls">
                <button type="button" class="primary" onclick="showScheduleMeetingModal()" aria-haspopup="dialog">
                    <span aria-hidden="true">📅</span> Schedule Meeting
                </button>
            </div>

            <div class="meeting-rooms-grid">
                <div class="meeting-room-card">
                    <h3>Active Meetings</h3>
                    <div id="active-meetings-list" class="meetings-list"></div>
                </div>
                <div class="meeting-room-card">
                    <h3>Upcoming Meetings</h3>
                    <div id="upcoming-meetings-list" class="meetings-list"></div>
                </div>
                <div class="meeting-room-card">
                    <h3>Past Meetings</h3>
                    <div id="past-meetings-list" class="meetings-list"></div>
                </div>
            </div>
        </div>
    `;

    container.innerHTML = html;
    renderMeetingsList();
}

function renderMeetingsList() {
    // Placeholder for rendering meetings
    const activeMeetings = document.getElementById('active-meetings-list');
    const upcomingMeetings = document.getElementById('upcoming-meetings-list');
    const pastMeetings = document.getElementById('past-meetings-list');

    if (activeMeetings) {
        activeMeetings.innerHTML = '<p class="empty-state">No active meetings</p>';
    }
    if (upcomingMeetings) {
        upcomingMeetings.innerHTML = '<p class="empty-state">No upcoming meetings</p>';
    }
    if (pastMeetings) {
        pastMeetings.innerHTML = '<p class="empty-state">No past meetings</p>';
    }
}

function showScheduleMeetingModal() {
    formModal({
        title: 'Schedule Meeting',
        submitText: 'Schedule',
        fields: [
            { id: 'meeting-title', label: 'Meeting Title', required: true, placeholder: 'e.g., Sprint Planning' },
            { id: 'meeting-description', label: 'Description', type: 'textarea', placeholder: 'Meeting agenda and details' },
            { id: 'meeting-agents', label: 'Invite Agents', required: true, placeholder: 'Select agents to invite' },
            { id: 'meeting-time', label: 'Start Time', type: 'datetime-local', required: true },
            { id: 'meeting-duration', label: 'Duration (minutes)', type: 'number', value: '60' }
        ]
    }).then(values => {
        if (values) {
            scheduleMeeting(values);
        }
    }).catch(err => {
        console.error('Schedule meeting error:', err);
    });
}

async function scheduleMeeting(values) {
    try {
        setBusy('schedule-meeting', true);
        const payload = {
            title: values['meeting-title'],
            description: values['meeting-description'],
            agents: values['meeting-agents'],
            start_time: values['meeting-time'],
            duration_minutes: parseInt(values['meeting-duration'], 10)
        };
        await apiCall('/meetings', {
            method: 'POST',
            body: JSON.stringify(payload)
        });
        showToast('Meeting scheduled successfully', 'success');
        scheduleReload('meetings', 150);
    } catch (error) {
        showToast(`Failed to schedule meeting: ${error.message}`, 'error');
    } finally {
        setBusy('schedule-meeting', false);
    }
}
