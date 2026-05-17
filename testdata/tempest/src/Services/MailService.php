<?php

declare(strict_types=1);

namespace App\Services;

use App\Models\User;

final class MailService {
    public function __construct(
        private readonly string $fromEmail = 'no-reply@example.com',
        private readonly string $fromName  = 'Example App',
    ) {}

    public function sendWelcome(User $user): bool
    {
        // Simulate sending an email
        return $this->send(
            to: $user->email,
            subject: "Welcome, {$user->name}!",
            body: $this->buildWelcomeBody($user),
        );
    }

    public function send(string $to, string $subject, string $body): bool
    {
        // In a real app this would use a mailer adapter
        return true;
    }

    private function buildWelcomeBody(User $user): string
    {
        return <<<HTML
        <html>
        <body>
            <h1>Welcome, {$user->name}!</h1>
            <p>Thank you for joining us.</p>
        </body>
        </html>
        HTML;
    }
}
