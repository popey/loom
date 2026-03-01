// Connectors Management JavaScript

// State management
let connectors = [];
let currentConnector = null;

// Load connectors on page load
async function loadConnectors() {
    const list = document.getElementById('connectors-list');
    if (!list) return;

    list.innerHTML = '<div class="loading">Loading connectors...</div>';

    try {
        const response = await fetch('/api/v1/connectors');
        if (!response.ok) throw new Error('Failed to load connectors');

        const data = await response.json();
        connectors = data.connectors || [];

        if (connectors.length === 0) {
            list.innerHTML = '<p class="empty-state">No connectors configured. Create one to get started.</p>';
            return;
        }

        list.innerHTML = '';
        connectors.forEach(connector => {
            const card = createConnectorCard(connector);
            list.appendChild(card);
        });

    } catch (err) {
        list.innerHTML = `<div class="error">Error loading connectors: ${err.message}</div>`;
    }
}

function createConnectorCard(connector) {
    const card = document.createElement('div');
    card.className = 'connector-card';
    card.style.cssText = 'padding: 1rem; border: 1px solid #ddd; border-radius: 4px; margin-bottom: 1rem; background: #f9f9f9;';

    const statusClass = connector.status === 'connected' ? 'connected' : 'disconnected';
    const statusIcon = connector.status === 'connected' ? '✓' : '✗';

    card.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: start;">
            <div style="flex: 1;">
                <h3 style="margin: 0 0 0.5rem 0;">${connector.name}</h3>
                <p style="margin: 0 0 0.5rem 0; color: #666; font-size: 0.9rem;">${connector.type}</p>
                <p style="margin: 0; color: #999; font-size: 0.85rem;">${connector.description || 'No description'}</p>
            </div>
            <div style="text-align: right;">
                <span class="status-badge ${statusClass}" style="display: inline-block; padding: 0.25rem 0.75rem; border-radius: 999px; font-size: 0.8rem; font-weight: 600; background: ${connector.status === 'connected' ? '#e8f5e9' : '#ffebee'}; color: ${connector.status === 'connected' ? '#2e7d32' : '#c62828'};">
                    ${statusIcon} ${connector.status}
                </span>
            </div>
        </div>
        <div style="margin-top: 1rem; display: flex; gap: 0.5rem;">
            <button type="button" class="secondary" onclick="editConnector('${connector.id}')" style="padding: 0.5rem 1rem; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer;">Edit</button>
            <button type="button" class="secondary" onclick="testConnector('${connector.id}')" style="padding: 0.5rem 1rem; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer;">Test</button>
            <button type="button" class="secondary" onclick="deleteConnector('${connector.id}')" style="padding: 0.5rem 1rem; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer; color: #d32f2f;">Delete</button>
        </div>
    `;

    return card;
}

function showCreateConnectorModal() {
    const modal = document.createElement('div');
    modal.id = 'create-connector-modal';
    modal.style.cssText = 'position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000;';

    modal.innerHTML = `
        <div style="background: white; padding: 2rem; border-radius: 8px; max-width: 500px; width: 90%;">
            <h2 style="margin-top: 0;">Create New Connector</h2>
            <form id="create-connector-form" style="display: flex; flex-direction: column; gap: 1rem;">
                <div>
                    <label for="connector-name" style="display: block; margin-bottom: 0.5rem; font-weight: 600;">Name *</label>
                    <input type="text" id="connector-name" required style="width: 100%; padding: 0.5rem; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box;">
                </div>
                <div>
                    <label for="connector-type" style="display: block; margin-bottom: 0.5rem; font-weight: 600;">Type *</label>
                    <select id="connector-type" required style="width: 100%; padding: 0.5rem; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box;">
                        <option value="">Select a type...</option>
                        <option value="http">HTTP/REST</option>
                        <option value="webhook">Webhook</option>
                        <option value="database">Database</option>
                        <option value="message_queue">Message Queue</option>
                        <option value="file_storage">File Storage</option>
                        <option value="custom">Custom</option>
                    </select>
                </div>
                <div>
                    <label for="connector-description" style="display: block; margin-bottom: 0.5rem; font-weight: 600;">Description</label>
                    <textarea id="connector-description" rows="3" style="width: 100%; padding: 0.5rem; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box;"></textarea>
                </div>
                <div style="display: flex; gap: 1rem; justify-content: flex-end;">
                    <button type="button" onclick="closeModal('create-connector-modal')" style="padding: 0.5rem 1rem; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer;">Cancel</button>
                    <button type="submit" style="padding: 0.5rem 1rem; background: #2196F3; color: white; border: none; border-radius: 4px; cursor: pointer;">Create</button>
                </div>
            </form>
        </div>
    `;

    document.body.appendChild(modal);

    document.getElementById('create-connector-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        await createConnector();
    });
}

async function createConnector() {
    const name = document.getElementById('connector-name').value;
    const type = document.getElementById('connector-type').value;
    const description = document.getElementById('connector-description').value;

    try {
        const response = await fetch('/api/v1/connectors', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, type, description })
        });

        if (!response.ok) throw new Error('Failed to create connector');

        closeModal('create-connector-modal');
        loadConnectors();
    } catch (err) {
        alert('Error creating connector: ' + err.message);
    }
}

function editConnector(connectorId) {
    const connector = connectors.find(c => c.id === connectorId);
    if (!connector) return;

    const modal = document.createElement('div');
    modal.id = 'edit-connector-modal';
    modal.style.cssText = 'position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000;';

    modal.innerHTML = `
        <div style="background: white; padding: 2rem; border-radius: 8px; max-width: 500px; width: 90%;">
            <h2 style="margin-top: 0;">Edit Connector</h2>
            <form id="edit-connector-form" style="display: flex; flex-direction: column; gap: 1rem;">
                <div>
                    <label for="edit-connector-name" style="display: block; margin-bottom: 0.5rem; font-weight: 600;">Name *</label>
                    <input type="text" id="edit-connector-name" value="${connector.name}" required style="width: 100%; padding: 0.5rem; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box;">
                </div>
                <div>
                    <label for="edit-connector-description" style="display: block; margin-bottom: 0.5rem; font-weight: 600;">Description</label>
                    <textarea id="edit-connector-description" rows="3" style="width: 100%; padding: 0.5rem; border: 1px solid #ddd; border-radius: 4px; box-sizing: border-box;">${connector.description || ''}</textarea>
                </div>
                <div style="display: flex; gap: 1rem; justify-content: flex-end;">
                    <button type="button" onclick="closeModal('edit-connector-modal')" style="padding: 0.5rem 1rem; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer;">Cancel</button>
                    <button type="submit" style="padding: 0.5rem 1rem; background: #2196F3; color: white; border: none; border-radius: 4px; cursor: pointer;">Save</button>
                </div>
            </form>
        </div>
    `;

    document.body.appendChild(modal);

    document.getElementById('edit-connector-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        await updateConnector(connectorId);
    });
}

async function updateConnector(connectorId) {
    const name = document.getElementById('edit-connector-name').value;
    const description = document.getElementById('edit-connector-description').value;

    try {
        const response = await fetch(`/api/v1/connectors/${connectorId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, description })
        });

        if (!response.ok) throw new Error('Failed to update connector');

        closeModal('edit-connector-modal');
        loadConnectors();
    } catch (err) {
        alert('Error updating connector: ' + err.message);
    }
}

async function testConnector(connectorId) {
    try {
        const response = await fetch(`/api/v1/connectors/${connectorId}/test`, {
            method: 'POST'
        });

        const data = await response.json();

        if (response.ok) {
            alert('Connector test successful!');
        } else {
            alert('Connector test failed: ' + (data.error || 'Unknown error'));
        }
    } catch (err) {
        alert('Error testing connector: ' + err.message);
    }
}

async function deleteConnector(connectorId) {
    if (!confirm('Are you sure you want to delete this connector?')) return;

    try {
        const response = await fetch(`/api/v1/connectors/${connectorId}`, {
            method: 'DELETE'
        });

        if (!response.ok) throw new Error('Failed to delete connector');

        loadConnectors();
    } catch (err) {
        alert('Error deleting connector: ' + err.message);
    }
}

function closeModal(modalId) {
    const modal = document.getElementById(modalId);
    if (modal) {
        modal.remove();
    }
}

// Load connectors when the page loads
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', loadConnectors);
} else {
    loadConnectors();
}
