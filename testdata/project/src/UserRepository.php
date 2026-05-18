<?php

namespace App;

/**
 * A generic repository. The type parameter records which model the
 * repository is scoped to.
 *
 * @template TModel of object
 */
class UserRepository
{
    /**
     * Build a repository scoped to the User model.
     *
     * @return UserRepository<User>
     */
    public static function forUsers()
    {
        return new self();
    }
}
